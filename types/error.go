package types

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param"`
	Code    any    `json:"code"`
}

type ClaudeError struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

type ErrorType string

const (
	ErrorTypeNewAPIError     ErrorType = "new_api_error"
	ErrorTypeOpenAIError     ErrorType = "openai_error"
	ErrorTypeClaudeError     ErrorType = "claude_error"
	ErrorTypeMidjourneyError ErrorType = "midjourney_error"
	ErrorTypeGeminiError     ErrorType = "gemini_error"
	ErrorTypeRerankError     ErrorType = "rerank_error"
	ErrorTypeUpstreamError   ErrorType = "upstream_error"
)

type ErrorCode string

const (
	ErrorCodeInvalidRequest         ErrorCode = "invalid_request"
	ErrorCodeSensitiveWordsDetected ErrorCode = "sensitive_words_detected"

	// new api error
	ErrorCodeCountTokenFailed   ErrorCode = "count_token_failed"
	ErrorCodeModelPriceError    ErrorCode = "model_price_error"
	ErrorCodeInvalidApiType     ErrorCode = "invalid_api_type"
	ErrorCodeJsonMarshalFailed  ErrorCode = "json_marshal_failed"
	ErrorCodeDoRequestFailed    ErrorCode = "do_request_failed"
	ErrorCodeGetChannelFailed   ErrorCode = "get_channel_failed"
	ErrorCodeGenRelayInfoFailed ErrorCode = "gen_relay_info_failed"

	// channel error
	ErrorCodeChannelNoAvailableKey        ErrorCode = "channel:no_available_key"
	ErrorCodeChannelParamOverrideInvalid  ErrorCode = "channel:param_override_invalid"
	ErrorCodeChannelHeaderOverrideInvalid ErrorCode = "channel:header_override_invalid"
	ErrorCodeChannelModelMappedError      ErrorCode = "channel:model_mapped_error"
	ErrorCodeChannelAwsClientError        ErrorCode = "channel:aws_client_error"
	ErrorCodeChannelInvalidKey            ErrorCode = "channel:invalid_key"
	ErrorCodeChannelResponseTimeExceeded  ErrorCode = "channel:response_time_exceeded"

	// client request error
	ErrorCodeReadRequestBodyFailed ErrorCode = "read_request_body_failed"
	ErrorCodeConvertRequestFailed  ErrorCode = "convert_request_failed"
	ErrorCodeAccessDenied          ErrorCode = "access_denied"

	// request error
	ErrorCodeBadRequestBody     ErrorCode = "bad_request_body"
	ErrorCodeGroupConfigInvalid ErrorCode = "group_config_invalid"
	ErrorCodeNoAvailableGroup   ErrorCode = "no_available_group"
	ErrorCodeAllGroupsFailed    ErrorCode = "all_groups_failed"

	// response error
	ErrorCodeReadResponseBodyFailed ErrorCode = "read_response_body_failed"
	ErrorCodeBadResponseStatusCode  ErrorCode = "bad_response_status_code"
	ErrorCodeBadResponse            ErrorCode = "bad_response"
	ErrorCodeBadResponseBody        ErrorCode = "bad_response_body"
	ErrorCodeEmptyResponse          ErrorCode = "empty_response"
	ErrorCodeAwsInvokeError         ErrorCode = "aws_invoke_error"
	ErrorCodeModelNotFound          ErrorCode = "model_not_found"
	ErrorCodePromptBlocked          ErrorCode = "prompt_blocked"

	// sql error
	ErrorCodeQueryDataError  ErrorCode = "query_data_error"
	ErrorCodeUpdateDataError ErrorCode = "update_data_error"

	// quota error
	ErrorCodeInsufficientUserQuota      ErrorCode = "insufficient_user_quota"
	ErrorCodePreConsumeTokenQuotaFailed ErrorCode = "pre_consume_token_quota_failed"
	ErrorCodeInsufficientBalance        ErrorCode = "insufficient_balance"

	// subscription error
	ErrorCodeSubscriptionNotFound            ErrorCode = "subscription_not_found"
	ErrorCodeSubscriptionExpired             ErrorCode = "subscription_expired"
	ErrorCodeSubscriptionNotActive           ErrorCode = "subscription_not_active"
	ErrorCodeSubscriptionLimitReached        ErrorCode = "subscription_limit_reached"
	ErrorCodeSubscriptionQuotaExhausted      ErrorCode = "subscription_quota_exhausted"
	ErrorCodeSubscriptionConflict            ErrorCode = "subscription_conflict"
	ErrorCodeSubscriptionCancelled           ErrorCode = "subscription_cancelled"
	ErrorCodeSubscriptionAutoRenewalFail     ErrorCode = "subscription_auto_renewal_fail"
	ErrorCodeSubscriptionOperationFailed     ErrorCode = "subscription_operation_failed"
	ErrorCodeSubscriptionPriorityConflict    ErrorCode = "subscription_priority_conflict"
	ErrorCodeSubscriptionInsufficientQuota   ErrorCode = "subscription_insufficient_quota"
	ErrorCodeSubscriptionPlanExpired         ErrorCode = "subscription_plan_expired"
	ErrorCodeSubscriptionPlanInactive        ErrorCode = "subscription_plan_inactive"

	// subscription plan error
	ErrorCodePlanNotFound           ErrorCode = "plan_not_found"
	ErrorCodePlanNotAvailable       ErrorCode = "plan_not_available"
	ErrorCodePlanNotPublished       ErrorCode = "plan_not_published"
	ErrorCodePlanInvalidStatus      ErrorCode = "plan_invalid_status"
	ErrorCodePlanLimitExceeded      ErrorCode = "plan_limit_exceeded"
	ErrorCodePlanPeriodDuplicate    ErrorCode = "plan_period_duplicate"
	ErrorCodePlanTimeConflict       ErrorCode = "plan_time_conflict"
	ErrorCodePlanSKUDuplicate       ErrorCode = "plan_sku_duplicate"
	ErrorCodePlanPriceInvalid       ErrorCode = "plan_price_invalid"
	ErrorCodePlanOperationFailed    ErrorCode = "plan_operation_failed"
	ErrorCodePlanModelInvalid       ErrorCode = "plan_model_invalid"       // 模型白名单包含无效模型
	ErrorCodePlanChannelInvalid     ErrorCode = "plan_channel_invalid"     // 渠道分组包含无效渠道

	// coupon error
	ErrorCodeCouponNotFound           ErrorCode = "coupon_not_found"
	ErrorCodeCouponExpired            ErrorCode = "coupon_expired"
	ErrorCodeCouponExhausted          ErrorCode = "coupon_exhausted"
	ErrorCodeCouponInvalidStatus      ErrorCode = "coupon_invalid_status"
	ErrorCodeCouponAlreadyUsed        ErrorCode = "coupon_already_used"
	ErrorCodeCouponBindingLocked      ErrorCode = "coupon_binding_locked"
	ErrorCodeCouponPlanMismatch       ErrorCode = "coupon_plan_mismatch"
	ErrorCodeCouponUserMismatch       ErrorCode = "coupon_user_mismatch"
	ErrorCodeCouponUsageLimitHit      ErrorCode = "coupon_usage_limit_hit"
	ErrorCodeCouponNotApplicable      ErrorCode = "coupon_not_applicable"
	ErrorCodeCouponReserved           ErrorCode = "coupon_reserved"
	ErrorCodeCouponCodeDuplicate      ErrorCode = "coupon_code_duplicate"
	ErrorCodeCouponOperationFailed    ErrorCode = "coupon_operation_failed"
	ErrorCodeCouponScopeMismatch      ErrorCode = "coupon_scope_mismatch"
	ErrorCodeCouponUserLimitReached   ErrorCode = "coupon_user_limit_reached"
	ErrorCodeCouponInsufficientQuota  ErrorCode = "coupon_insufficient_quota"

	// subscription order error
	ErrorCodeOrderNotFound        ErrorCode = "order_not_found"
	ErrorCodeOrderInvalidStatus   ErrorCode = "order_invalid_status"
	ErrorCodeOrderAlreadyPaid     ErrorCode = "order_already_paid"
	ErrorCodeOrderExpired         ErrorCode = "order_expired"
	ErrorCodeOrderPaymentFailed   ErrorCode = "order_payment_failed"
	ErrorCodeOrderCancelled       ErrorCode = "order_cancelled"
	ErrorCodeOrderCreationFailed  ErrorCode = "order_creation_failed"
	ErrorCodeOrderInvalidAmount   ErrorCode = "order_invalid_amount"
	ErrorCodeOrderAlreadyRefunded ErrorCode = "order_already_refunded"

	// subscription usage error
	ErrorCodeUsageRecordFailed    ErrorCode = "usage_record_failed"
	ErrorCodeUsageQuotaExceeded   ErrorCode = "usage_quota_exceeded"
	ErrorCodeUsageInvalidType     ErrorCode = "usage_invalid_type"

	// user bill error
	ErrorCodeBillNotFound         ErrorCode = "bill_not_found"
	ErrorCodeBillGenerationFailed ErrorCode = "bill_generation_failed"
	ErrorCodeBillAlreadyPaid      ErrorCode = "bill_already_paid"
	ErrorCodeBillOperationFailed  ErrorCode = "bill_operation_failed"

	// redemption error (extended)
	ErrorCodeRedemptionConflict       ErrorCode = "redemption_conflict"
	ErrorCodeRedemptionInvalidType    ErrorCode = "redemption_invalid_type"
	ErrorCodeRedemptionExpired        ErrorCode = "redemption_expired"
	ErrorCodeRedemptionUsed           ErrorCode = "redemption_used"
	ErrorCodeRedemptionInvalid        ErrorCode = "redemption_invalid"
	ErrorCodeRedemptionNotFound       ErrorCode = "redemption_not_found"
	ErrorCodeRedemptionOperationFailed ErrorCode = "redemption_operation_failed"

	// validation and request error
	ErrorCodeInvalidRequestParams       ErrorCode = "invalid_request_params"
	ErrorCodeSubscriptionMaxLimitReached ErrorCode = "subscription_max_limit_reached"
)

type NewAPIError struct {
	Err            error
	RelayError     any
	skipRetry      bool
	recordErrorLog *bool
	errorType      ErrorType
	errorCode      ErrorCode
	StatusCode     int
}

func (e *NewAPIError) GetErrorCode() ErrorCode {
	if e == nil {
		return ""
	}
	return e.errorCode
}

func (e *NewAPIError) GetErrorType() ErrorType {
	if e == nil {
		return ""
	}
	return e.errorType
}

func (e *NewAPIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		// fallback message when underlying error is missing
		return string(e.errorCode)
	}
	return e.Err.Error()
}

func (e *NewAPIError) MaskSensitiveError() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.errorCode)
	}
	errStr := e.Err.Error()
	if e.errorCode == ErrorCodeCountTokenFailed {
		return errStr
	}
	return common.MaskSensitiveInfo(errStr)
}

func (e *NewAPIError) SetMessage(message string) {
	e.Err = errors.New(message)
}

func (e *NewAPIError) ToOpenAIError() OpenAIError {
	var result OpenAIError
	switch e.errorType {
	case ErrorTypeOpenAIError:
		if openAIError, ok := e.RelayError.(OpenAIError); ok {
			result = openAIError
		}
	case ErrorTypeClaudeError:
		if claudeError, ok := e.RelayError.(ClaudeError); ok {
			result = OpenAIError{
				Message: e.Error(),
				Type:    claudeError.Type,
				Param:   "",
				Code:    e.errorCode,
			}
		}
	default:
		result = OpenAIError{
			Message: e.Error(),
			Type:    string(e.errorType),
			Param:   "",
			Code:    e.errorCode,
		}
	}
	if e.errorCode != ErrorCodeCountTokenFailed {
		result.Message = common.MaskSensitiveInfo(result.Message)
	}
	if result.Message == "" {
		result.Message = string(e.errorType)
	}
	return result
}

func (e *NewAPIError) ToClaudeError() ClaudeError {
	var result ClaudeError
	switch e.errorType {
	case ErrorTypeOpenAIError:
		if openAIError, ok := e.RelayError.(OpenAIError); ok {
			result = ClaudeError{
				Message: e.Error(),
				Type:    fmt.Sprintf("%v", openAIError.Code),
			}
		}
	case ErrorTypeClaudeError:
		if claudeError, ok := e.RelayError.(ClaudeError); ok {
			result = claudeError
		}
	default:
		result = ClaudeError{
			Message: e.Error(),
			Type:    string(e.errorType),
		}
	}
	if e.errorCode != ErrorCodeCountTokenFailed {
		result.Message = common.MaskSensitiveInfo(result.Message)
	}
	if result.Message == "" {
		result.Message = string(e.errorType)
	}
	return result
}

type NewAPIErrorOptions func(*NewAPIError)

func NewError(err error, errorCode ErrorCode, ops ...NewAPIErrorOptions) *NewAPIError {
	var newErr *NewAPIError
	// 保留深层传递的 new err
	if errors.As(err, &newErr) {
		for _, op := range ops {
			op(newErr)
		}
		return newErr
	}
	e := &NewAPIError{
		Err:        err,
		RelayError: nil,
		errorType:  ErrorTypeNewAPIError,
		StatusCode: http.StatusInternalServerError,
		errorCode:  errorCode,
	}
	for _, op := range ops {
		op(e)
	}
	return e
}

func NewOpenAIError(err error, errorCode ErrorCode, statusCode int, ops ...NewAPIErrorOptions) *NewAPIError {
	var newErr *NewAPIError
	// 保留深层传递的 new err
	if errors.As(err, &newErr) {
		if newErr.RelayError == nil {
			openaiError := OpenAIError{
				Message: newErr.Error(),
				Type:    string(errorCode),
				Code:    errorCode,
			}
			newErr.RelayError = openaiError
		}
		for _, op := range ops {
			op(newErr)
		}
		return newErr
	}
	openaiError := OpenAIError{
		Message: err.Error(),
		Type:    string(errorCode),
		Code:    errorCode,
	}
	return WithOpenAIError(openaiError, statusCode, ops...)
}

func InitOpenAIError(errorCode ErrorCode, statusCode int, ops ...NewAPIErrorOptions) *NewAPIError {
	openaiError := OpenAIError{
		Type: string(errorCode),
		Code: errorCode,
	}
	return WithOpenAIError(openaiError, statusCode, ops...)
}

func NewErrorWithStatusCode(err error, errorCode ErrorCode, statusCode int, ops ...NewAPIErrorOptions) *NewAPIError {
	e := &NewAPIError{
		Err: err,
		RelayError: OpenAIError{
			Message: err.Error(),
			Type:    string(errorCode),
		},
		errorType:  ErrorTypeNewAPIError,
		StatusCode: statusCode,
		errorCode:  errorCode,
	}
	for _, op := range ops {
		op(e)
	}

	return e
}

func WithOpenAIError(openAIError OpenAIError, statusCode int, ops ...NewAPIErrorOptions) *NewAPIError {
	code, ok := openAIError.Code.(string)
	if !ok {
		if openAIError.Code != nil {
			code = fmt.Sprintf("%v", openAIError.Code)
		} else {
			code = "unknown_error"
		}
	}
	if openAIError.Type == "" {
		openAIError.Type = "upstream_error"
	}
	e := &NewAPIError{
		RelayError: openAIError,
		errorType:  ErrorTypeOpenAIError,
		StatusCode: statusCode,
		Err:        errors.New(openAIError.Message),
		errorCode:  ErrorCode(code),
	}
	for _, op := range ops {
		op(e)
	}
	return e
}

func WithClaudeError(claudeError ClaudeError, statusCode int, ops ...NewAPIErrorOptions) *NewAPIError {
	if claudeError.Type == "" {
		claudeError.Type = "upstream_error"
	}
	e := &NewAPIError{
		RelayError: claudeError,
		errorType:  ErrorTypeClaudeError,
		StatusCode: statusCode,
		Err:        errors.New(claudeError.Message),
		errorCode:  ErrorCode(claudeError.Type),
	}
	for _, op := range ops {
		op(e)
	}
	return e
}

func IsChannelError(err *NewAPIError) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(string(err.errorCode), "channel:")
}

func IsSkipRetryError(err *NewAPIError) bool {
	if err == nil {
		return false
	}

	return err.skipRetry
}

func ErrOptionWithSkipRetry() NewAPIErrorOptions {
	return func(e *NewAPIError) {
		e.skipRetry = true
	}
}

func ErrOptionWithNoRecordErrorLog() NewAPIErrorOptions {
	return func(e *NewAPIError) {
		e.recordErrorLog = common.GetPointer(false)
	}
}

func ErrOptionWithHideErrMsg(replaceStr string) NewAPIErrorOptions {
	return func(e *NewAPIError) {
		if common.DebugEnabled {
			fmt.Printf("ErrOptionWithHideErrMsg: %s, origin error: %s", replaceStr, e.Err)
		}
		e.Err = errors.New(replaceStr)
	}
}

func IsRecordErrorLog(e *NewAPIError) bool {
	if e == nil {
		return false
	}
	if e.recordErrorLog == nil {
		// default to true if not set
		return true
	}
	return *e.recordErrorLog
}
