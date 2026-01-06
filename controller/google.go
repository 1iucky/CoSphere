package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type GoogleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	IdToken      string `json:"id_token"`
}

type GoogleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

const (
	googleTokenEndpoint    = "https://oauth2.googleapis.com/token"
	googleUserInfoEndpoint = "https://openidconnect.googleapis.com/v1/userinfo"
)

func getGoogleUserInfoByCode(code string) (*GoogleUserInfo, error) {
	if code == "" {
		return nil, errors.New("未获取到 Google 授权码")
	}
	settings := system_setting.GetGoogleSettings()
	if settings.ClientId == "" || settings.ClientSecret == "" {
		return nil, errors.New("管理员尚未配置 Google OAuth Client")
	}
	if system_setting.ServerAddress == "" {
		return nil, errors.New("管理员尚未配置 ServerAddress，无法使用 Google 登录")
	}

	values := url.Values{}
	values.Set("code", code)
	values.Set("client_id", settings.ClientId)
	values.Set("client_secret", settings.ClientSecret)
	values.Set("redirect_uri", fmt.Sprintf("%s/oauth/google", system_setting.ServerAddress))
	values.Set("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", googleTokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		common.SysLog("google token exchange failed: " + err.Error())
		return nil, errors.New("无法连接至 Google 服务器，请稍后重试！")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		common.SysLog(fmt.Sprintf("google token exchange failed, status=%d body=%s", res.StatusCode, string(body)))
		return nil, errors.New("Google 授权失败，请稍后重试！")
	}

	var token GoogleTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}
	if token.AccessToken == "" {
		return nil, errors.New("Google 返回的 access token 为空")
	}

	userReq, err := http.NewRequest("GET", googleUserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userRes, err := client.Do(userReq)
	if err != nil {
		common.SysLog("google userinfo request failed: " + err.Error())
		return nil, errors.New("无法连接至 Google 服务器，请稍后重试！")
	}
	defer userRes.Body.Close()

	userBody, err := io.ReadAll(userRes.Body)
	if err != nil {
		return nil, err
	}
	if userRes.StatusCode != http.StatusOK {
		common.SysLog(fmt.Sprintf("google userinfo failed, status=%d body=%s", userRes.StatusCode, string(userBody)))
		return nil, errors.New("Google 用户信息获取失败，请稍后重试！")
	}

	var info GoogleUserInfo
	if err := json.Unmarshal(userBody, &info); err != nil {
		return nil, err
	}
	if info.Sub == "" {
		return nil, errors.New("Google 用户信息缺少标识")
	}
	if info.Email == "" {
		return nil, errors.New("Google 用户邮箱为空，请检查授权范围")
	}
	if !info.EmailVerified {
		return nil, errors.New("Google 邮箱尚未验证，无法登录")
	}
	return &info, nil
}

func GoogleOAuth(c *gin.Context) {
	session := sessions.Default(c)
	state := c.Query("state")
	if state == "" || session.Get("oauth_state") == nil || state != session.Get("oauth_state").(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "state is empty or not same",
		})
		return
	}

	if session.Get("username") != nil {
		GoogleBind(c)
		return
	}

	if !system_setting.GetGoogleSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Google 登录以及注册",
		})
		return
	}

	code := c.Query("code")
	googleUser, err := getGoogleUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	user := model.User{
		GoogleId: googleUser.Sub,
	}
	if model.IsGoogleIdAlreadyTaken(user.GoogleId) {
		if err := user.FillUserByGoogleId(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		if user.Id == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "用户已注销",
			})
			return
		}
	} else {
		if !common.RegisterEnabled {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员关闭了新用户注册",
			})
			return
		}
		user.Username = "google_" + strconv.Itoa(model.GetMaxUserId()+1)
		if googleUser.Name != "" {
			user.DisplayName = googleUser.Name
		} else {
			user.DisplayName = "Google User"
		}
		user.Email = googleUser.Email
		user.Role = common.RoleCommonUser
		user.Status = common.UserStatusEnabled

		affCode := session.Get("aff")
		inviterId := 0
		if affCode != nil {
			inviterId, _ = model.GetUserIdByAffCode(affCode.(string))
		}

		if err := user.Insert(inviterId); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已被封禁",
			"success": false,
		})
		return
	}

	setupLogin(&user, c)
}

func GoogleBind(c *gin.Context) {
	if !system_setting.GetGoogleSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Google 登录以及注册",
		})
		return
	}

	code := c.Query("code")
	googleUser, err := getGoogleUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	user := model.User{
		GoogleId: googleUser.Sub,
	}
	if model.IsGoogleIdAlreadyTaken(user.GoogleId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 Google 账户已被绑定",
		})
		return
	}

	session := sessions.Default(c)
	id := session.Get("id")
	user.Id = id.(int)
	if err := user.FillUserById(); err != nil {
		common.ApiError(c, err)
		return
	}

	user.GoogleId = googleUser.Sub
	if err := user.Update(false); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "bind",
	})
}
