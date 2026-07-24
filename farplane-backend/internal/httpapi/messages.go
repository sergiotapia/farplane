package httpapi

// Shared JSON keys and API error messages. Keeps goconst green and call sites consistent.
const (
	jsonKeyError          = "error"
	jsonKeyStatus         = "status"
	jsonKeyType           = "type"
	jsonKeyEvent          = "event"
	jsonKeyName           = "name"
	jsonKeyEmail          = "email"
	jsonKeyUserID         = "user_id"
	jsonKeyLaneID         = "lane_id"
	jsonKeyOrganizationID = "organization_id"
	jsonKeyDisplayName    = "display_name"
	jsonKeyCreatedAt      = "created_at"
	jsonKeyUpdatedAt      = "updated_at"
	jsonKeyPartial        = "partial"

	errAuthRequired        = "authentication required"
	errDatabaseUnavailable = "database unavailable"
	errInvalidRequestBody  = "invalid request body"
	errNotFound            = "not found"
	errSetupStatus         = "failed to read setup status"
	errPasswordTooLong     = "password must be at most 72 bytes"
	errModelSource         = "model_source not available for this agent"
	fieldLastValidationLog = "last_validation_log"
)
