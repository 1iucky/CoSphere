create table public.abilities (
  "group" character varying(64) not null,
  model character varying(255) not null,
  channel_id bigint not null,
  enabled boolean,
  priority bigint default 0,
  weight bigint default 0,
  tag text,
  primary key ("group", model, channel_id)
);
create index idx_abilities_tag on abilities using btree (tag);
create index idx_abilities_weight on abilities using btree (weight);
create index idx_abilities_priority on abilities using btree (priority);
create index idx_abilities_channel_id on abilities using btree (channel_id);

create table public.channels (
  id bigint primary key not null default nextval('channels_id_seq'::regclass),
  type bigint default 0,
  key text not null,
  open_ai_organization text,
  test_model text,
  status bigint default 1,
  name text,
  weight bigint default 0,
  created_time bigint,
  test_time bigint,
  response_time bigint,
  base_url text default ''::text,
  other text,
  balance numeric,
  balance_updated_time bigint,
  models text,
  "group" character varying(64) default 'default',
  used_quota bigint default 0,
  model_mapping text,
  status_code_mapping character varying(1024) default '',
  priority bigint default 0,
  auto_ban bigint default 1,
  other_info text,
  tag text,
  setting text,
  param_override text,
  header_override text,
  remark character varying(255),
  channel_info json,
  settings text
);
create index idx_channels_tag on channels using btree (tag);
create index idx_channels_name on channels using btree (name);

create table public.logs (
  id bigint primary key not null default nextval('logs_id_seq'::regclass),
  user_id bigint,
  created_at bigint,
  type bigint,
  content text,
  username text default ''::text,
  token_name text default ''::text,
  model_name text default ''::text,
  quota bigint default 0,
  prompt_tokens bigint default 0,
  completion_tokens bigint default 0,
  use_time bigint default 0,
  is_stream boolean,
  channel_id bigint,
  channel_name text,
  token_id bigint default 0,
  "group" text,
  ip text default ''::text,
  other text
);
create index idx_logs_token_id on logs using btree (token_id);
create index idx_logs_model_name on logs using btree (model_name);
create index idx_created_at_id on logs using btree (id, created_at);
create index idx_logs_ip on logs using btree (ip);
create index idx_logs_group on logs using btree ("group");
create index idx_logs_channel_id on logs using btree (channel_id);
create index idx_logs_token_name on logs using btree (token_name);
create index index_username_model_name on logs using btree (model_name, username);
create index idx_logs_username on logs using btree (username);
create index idx_created_at_type on logs using btree (created_at, type);
create index idx_logs_user_id on logs using btree (user_id);

create table public.midjourneys (
  id bigint primary key not null default nextval('midjourneys_id_seq'::regclass),
  code bigint,
  user_id bigint,
  action character varying(40),
  mj_id text,
  prompt text,
  prompt_en text,
  description text,
  state text,
  submit_time bigint,
  start_time bigint,
  finish_time bigint,
  image_url text,
  video_url text,
  video_urls text,
  status character varying(20),
  progress character varying(30),
  fail_reason text,
  channel_id bigint,
  quota bigint,
  buttons text,
  properties text
);
create index idx_midjourneys_status on midjourneys using btree (status);
create index idx_midjourneys_finish_time on midjourneys using btree (finish_time);
create index idx_midjourneys_start_time on midjourneys using btree (start_time);
create index idx_midjourneys_submit_time on midjourneys using btree (submit_time);
create index idx_midjourneys_mj_id on midjourneys using btree (mj_id);
create index idx_midjourneys_action on midjourneys using btree (action);
create index idx_midjourneys_user_id on midjourneys using btree (user_id);
create index idx_midjourneys_progress on midjourneys using btree (progress);

create table public.models (
  id bigint primary key not null default nextval('models_id_seq'::regclass),
  model_name character varying(128) not null,
  description text,
  icon character varying(128),
  tags character varying(255),
  vendor_id bigint,
  endpoints text,
  status bigint default 1,
  sync_official bigint default 1,
  created_time bigint,
  updated_time bigint,
  deleted_at timestamp with time zone,
  name_rule bigint default 0
);
create index idx_models_deleted_at on models using btree (deleted_at);
create index idx_models_vendor_id on models using btree (vendor_id);
create unique index uk_model_name_delete_at on models using btree (model_name, deleted_at);

create table public.options (
  key text primary key not null,
  value text
);

create table public.passkey_credentials (
  id bigint primary key not null default nextval('passkey_credentials_id_seq'::regclass),
  user_id bigint not null,
  credential_id character varying(512) not null,
  public_key text not null,
  attestation_type character varying(255),
  aa_guid character varying(512),
  sign_count bigint default 0,
  clone_warning boolean,
  user_present boolean,
  user_verified boolean,
  backup_eligible boolean,
  backup_state boolean,
  transports text,
  attachment character varying(32),
  last_used_at timestamp with time zone,
  created_at timestamp with time zone,
  updated_at timestamp with time zone,
  deleted_at timestamp with time zone
);
create unique index idx_passkey_credentials_user_id on passkey_credentials using btree (user_id);
create index idx_passkey_credentials_deleted_at on passkey_credentials using btree (deleted_at);
create unique index idx_passkey_credentials_credential_id on passkey_credentials using btree (credential_id);

create table public.prefill_groups (
  id bigint primary key not null default nextval('prefill_groups_id_seq'::regclass),
  name character varying(64) not null,
  type character varying(32) not null,
  items json,
  description character varying(255),
  created_time bigint,
  updated_time bigint,
  deleted_at timestamp with time zone
);
create index idx_prefill_groups_deleted_at on prefill_groups using btree (deleted_at);
create index idx_prefill_groups_type on prefill_groups using btree (type);
create unique index uk_prefill_name on prefill_groups using btree (name) WHERE (deleted_at IS NULL);
create unique index idx_prefill_groups_name on prefill_groups using btree (name);

create table public.quota_data (
  id bigint primary key not null default nextval('quota_data_id_seq'::regclass),
  user_id bigint,
  username character varying(64) default '',
  model_name character varying(64) default '',
  created_at bigint,
  token_used bigint default 0,
  count bigint default 0,
  quota bigint default 0
);
create index idx_quota_data_user_id on quota_data using btree (user_id);
create index idx_qdt_created_at on quota_data using btree (created_at);
create index idx_qdt_model_user_name on quota_data using btree (model_name, username);

create table public.redemptions (
  id bigint primary key not null default nextval('redemptions_id_seq'::regclass),
  user_id bigint,
  key character(32),
  status bigint default 1,
  name text,
  quota bigint default 100,
  created_time bigint,
  redeemed_time bigint,
  used_user_id bigint,
  deleted_at timestamp with time zone,
  expired_time bigint
);
create index idx_redemptions_deleted_at on redemptions using btree (deleted_at);
create index idx_redemptions_name on redemptions using btree (name);
create unique index idx_redemptions_key on redemptions using btree (key);

create table public.setups (
  id bigint primary key not null default nextval('setups_id_seq'::regclass),
  version character varying(50) not null,
  initialized_at bigint not null
);

create table public.tasks (
  id bigint primary key not null default nextval('tasks_id_seq'::regclass),
  created_at bigint,
  updated_at bigint,
  task_id character varying(191),
  platform character varying(30),
  user_id bigint,
  "group" character varying(50),
  channel_id bigint,
  quota bigint,
  action character varying(40),
  status character varying(20),
  fail_reason text,
  submit_time bigint,
  start_time bigint,
  finish_time bigint,
  progress character varying(20),
  properties json,
  private_data json,
  data json
);
create index idx_tasks_submit_time on tasks using btree (submit_time);
create index idx_tasks_status on tasks using btree (status);
create index idx_tasks_action on tasks using btree (action);
create index idx_tasks_user_id on tasks using btree (user_id);
create index idx_tasks_platform on tasks using btree (platform);
create index idx_tasks_task_id on tasks using btree (task_id);
create index idx_tasks_created_at on tasks using btree (created_at);
create index idx_tasks_progress on tasks using btree (progress);
create index idx_tasks_start_time on tasks using btree (start_time);
create index idx_tasks_channel_id on tasks using btree (channel_id);
create index idx_tasks_finish_time on tasks using btree (finish_time);

create table public.tokens (
  id bigint primary key not null default nextval('tokens_id_seq'::regclass),
  user_id bigint,
  key character(48),
  status bigint default 1,
  name text,
  created_time bigint,
  accessed_time bigint,
  expired_time bigint default '-1'::integer,
  remain_quota bigint default 0,
  unlimited_quota boolean,
  model_limits_enabled boolean,
  model_limits character varying(1024) default '',
  allow_ips text default ''::text,
  used_quota bigint default 0,
  "group" text default ''::text,
  deleted_at timestamp with time zone,
  group_priorities character varying(2048) default '',
  auto_smart_group boolean default false,
  subscription_preferred boolean default false
);
create index idx_tokens_deleted_at on tokens using btree (deleted_at);
create index idx_tokens_name on tokens using btree (name);
create index idx_tokens_user_id on tokens using btree (user_id);
create unique index idx_tokens_key on tokens using btree (key);

create table public.top_ups (
  id bigint primary key not null default nextval('top_ups_id_seq'::regclass),
  user_id bigint,
  amount bigint,
  money numeric,
  trade_no character varying(255),
  payment_method character varying(50),
  create_time bigint,
  complete_time bigint,
  status text
);
create unique index top_ups_trade_no_key on top_ups using btree (trade_no);
create index idx_top_ups_trade_no on top_ups using btree (trade_no);
create index idx_top_ups_user_id on top_ups using btree (user_id);

create table public.two_fa_backup_codes (
  id bigint primary key not null default nextval('two_fa_backup_codes_id_seq'::regclass),
  user_id bigint not null,
  code_hash character varying(255) not null,
  is_used boolean,
  used_at timestamp with time zone,
  created_at timestamp with time zone,
  deleted_at timestamp with time zone
);
create index idx_two_fa_backup_codes_deleted_at on two_fa_backup_codes using btree (deleted_at);
create index idx_two_fa_backup_codes_user_id on two_fa_backup_codes using btree (user_id);

create table public.two_fas (
  id bigint primary key not null default nextval('two_fas_id_seq'::regclass),
  user_id bigint not null,
  secret character varying(255) not null,
  is_enabled boolean,
  failed_attempts bigint default 0,
  locked_until timestamp with time zone,
  last_used_at timestamp with time zone,
  created_at timestamp with time zone,
  updated_at timestamp with time zone,
  deleted_at timestamp with time zone
);
create unique index two_fas_user_id_key on two_fas using btree (user_id);
create index idx_two_fas_deleted_at on two_fas using btree (deleted_at);
create index idx_two_fas_user_id on two_fas using btree (user_id);

create table public.users (
  id bigint primary key not null default nextval('users_id_seq'::regclass),
  username text,
  password text not null,
  display_name text,
  role bigint default 1,
  status bigint default 1,
  email text,
  github_id text,
  discord_id text,
  oidc_id text,
  wechat_id text,
  telegram_id text,
  access_token character(32),
  quota bigint default 0,
  used_quota bigint default 0,
  request_count bigint default 0,
  "group" character varying(64) default 'default',
  aff_code character varying(32),
  aff_count bigint default 0,
  aff_quota bigint default 0,
  aff_history bigint default 0,
  inviter_id bigint,
  deleted_at timestamp with time zone,
  linux_do_id text,
  setting text,
  remark character varying(255),
  stripe_customer character varying(64),
  google_id text
);
create unique index users_username_key on users using btree (username);
create index idx_users_discord_id on users using btree (discord_id);
create index idx_users_telegram_id on users using btree (telegram_id);
create index idx_users_we_chat_id on users using btree (wechat_id);
create index idx_users_git_hub_id on users using btree (github_id);
create index idx_users_email on users using btree (email);
create index idx_users_display_name on users using btree (display_name);
create index idx_users_username on users using btree (username);
create index idx_users_stripe_customer on users using btree (stripe_customer);
create index idx_users_linux_do_id on users using btree (linux_do_id);
create index idx_users_deleted_at on users using btree (deleted_at);
create index idx_users_inviter_id on users using btree (inviter_id);
create index idx_users_oidc_id on users using btree (oidc_id);
create index idx_users_google_id on users using btree (google_id);
create unique index idx_users_access_token on users using btree (access_token);
create unique index idx_users_aff_code on users using btree (aff_code);

create table public.vendors (
  id bigint primary key not null default nextval('vendors_id_seq'::regclass),
  name character varying(128) not null,
  description text,
  icon character varying(128),
  status bigint default 1,
  created_time bigint,
  updated_time bigint,
  deleted_at timestamp with time zone
);
create unique index uk_vendor_name_delete_at on vendors using btree (name, deleted_at);
create index idx_vendors_deleted_at on vendors using btree (deleted_at);
