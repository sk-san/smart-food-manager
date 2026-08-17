package logging

// Event categories (event.category).
const (
	CategoryUX         = "ux"
	CategoryHTTP       = "http"
	CategoryAuth       = "auth"
	CategoryDependency = "dependency"
	CategoryFile       = "file"
	CategoryAccount    = "account"
)

// Event outcomes (event.outcome).
const (
	OutcomeStarted = "started"
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

// Backend event catalog. Event names are stable identifiers: dashboards
// and alerts key on them, so treat renames as breaking changes.
const (
	EventRequestReceived  = "request_received"
	EventRequestCompleted = "request_completed"
	EventRequestFailed    = "request_failed"

	EventGuestAIQuotaExceeded = "guest_ai_quota_exceeded"

	EventAuthLoginStarted    = "auth_login_started"
	EventAuthLoginCompleted  = "auth_login_completed"
	EventAuthLoginFailed     = "auth_login_failed"
	EventAuthLogoutCompleted = "auth_logout_completed"

	EventAccountDeleteRequested = "user_account_delete_requested"
	EventAccountDeleteCompleted = "user_account_delete_completed"
	EventAccountDeleteFailed    = "user_account_delete_failed"

	EventFileUploadStarted   = "file_upload_started"
	EventFileUploadCompleted = "file_upload_completed"
	EventFileUploadFailed    = "file_upload_failed"

	EventExternalAPICallStarted   = "external_api_call_started"
	EventExternalAPICallCompleted = "external_api_call_completed"
	EventExternalAPICallFailed    = "external_api_call_failed"
)

// FrontendEventCategories is the frontend event catalog: accepted event
// names mapped to their canonical category. The telemetry endpoint drops
// events whose names are not in this set, so the catalog doubles as the
// ingestion allowlist.
var FrontendEventCategories = map[string]string{
	"screen_view":     CategoryUX,
	"navigation":      CategoryUX,
	"screen_rendered": CategoryUX,
	"frontend_error":  CategoryUX,

	"api_request_started":   CategoryHTTP,
	"api_request_completed": CategoryHTTP,
	"api_request_failed":    CategoryHTTP,

	"login_clicked":  CategoryAuth,
	"logout_clicked": CategoryAuth,

	"delete_account_requested": CategoryAccount,
	"delete_account_confirmed": CategoryAccount,

	"file_upload_started":   CategoryFile,
	"file_upload_completed": CategoryFile,
	"file_upload_failed":    CategoryFile,
}
