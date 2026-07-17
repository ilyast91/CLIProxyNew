package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

const (
	maxOAuthCredentialImportBytes    = 1 << 20
	maxOAuthCredentialMultipartBytes = maxOAuthCredentialImportBytes + (64 << 10)
)

var (
	errOAuthCredentialDuplicate        = errors.New("OAuth credential already exists")
	errOAuthCredentialTooLarge         = errors.New("OAuth credential is too large")
	errOAuthCredentialUnsupportedMedia = errors.New("unsupported OAuth credential content type")
)

type oauthCredentialManager interface {
	GetByID(string) (*coreauth.Auth, bool)
	List() []*coreauth.Auth
	Register(context.Context, *coreauth.Auth) (*coreauth.Auth, error)
}

type adminAuditLogger interface {
	Insert(context.Context, store.AdminAuditLogEntry) error
}

type importedOAuthCredentialResponse struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Email    string `json:"email"`
}

// AdminOAuthCredentialHandler экспортирует и импортирует чувствительные OAuth credentials.
type AdminOAuthCredentialHandler struct {
	manager oauthCredentialManager
	audit   adminAuditLogger
}

// NewAdminOAuthCredentialHandler создаёт handler OAuth export/import.
func NewAdminOAuthCredentialHandler(manager oauthCredentialManager, audit adminAuditLogger) *AdminOAuthCredentialHandler {
	return &AdminOAuthCredentialHandler{manager: manager, audit: audit}
}

// Export отдаёт полный OAuth Auth JSON как attachment и пишет audit-запись.
func (h *AdminOAuthCredentialHandler) Export(c *gin.Context) {
	actor, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.manager == nil || h.audit == nil {
		writeError(c, http.StatusInternalServerError, "OAuth credential service is unavailable")
		return
	}
	auth, ok := h.manager.GetByID(strings.TrimSpace(c.Param("accountID")))
	if !ok || auth == nil {
		writeError(c, http.StatusNotFound, "upstream account not found")
		return
	}
	if auth.AuthKind() != coreauth.AuthKindOAuth {
		writeError(c, http.StatusBadRequest, "upstream account is not OAuth")
		return
	}
	_, email := auth.AccountInfo()
	details, _ := json.Marshal(map[string]string{"provider": auth.Provider, "email": email})
	if err := h.audit.Insert(c.Request.Context(), store.AdminAuditLogEntry{ActorUserID: actor, Action: "oauth.credential.exported", TargetType: "upstream_account", TargetID: auth.ID, Details: details, ActorIP: requestActorIP(c)}); err != nil {
		writeError(c, http.StatusInternalServerError, "write OAuth export audit failed")
		return
	}
	payload, err := json.Marshal(auth)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "encode OAuth credential failed")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="oauth-credential.json"`)
	c.Data(http.StatusOK, "application/json", payload)
}

// Import принимает полный OAuth Auth как JSON body или multipart JSON-файл,
// отклоняет provider/email дубликаты и регистрирует credential.
func (h *AdminOAuthCredentialHandler) Import(c *gin.Context) {
	actor, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.manager == nil {
		writeError(c, http.StatusInternalServerError, "OAuth credential service is unavailable")
		return
	}
	auth, err := decodeOAuthCredentialRequest(c)
	if errors.Is(err, errOAuthCredentialTooLarge) {
		writeError(c, http.StatusRequestEntityTooLarge, errOAuthCredentialTooLarge.Error())
		return
	}
	if errors.Is(err, errOAuthCredentialUnsupportedMedia) {
		writeError(c, http.StatusUnsupportedMediaType, errOAuthCredentialUnsupportedMedia.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid OAuth credential")
		return
	}
	provider := strings.TrimSpace(auth.Provider)
	_, email := auth.AccountInfo()
	email = strings.TrimSpace(email)
	if provider == "" || email == "" || auth.AuthKind() != coreauth.AuthKindOAuth {
		writeError(c, http.StatusBadRequest, "invalid OAuth credential")
		return
	}
	if h.hasDuplicate(provider, email) {
		writeError(c, http.StatusConflict, errOAuthCredentialDuplicate.Error())
		return
	}
	// ID выдаёт Manager.Register, чтобы импорт не переносил внешний runtime ID.
	auth.ID = ""
	details, _ := json.Marshal(map[string]string{"provider": provider, "email": email})
	ctx := store.WithUpstreamAccountAudit(c.Request.Context(), store.AdminAuditLogEntry{ActorUserID: actor, Action: "oauth.credential.imported", TargetType: "upstream_account", TargetID: "pending", Details: details, ActorIP: requestActorIP(c)})
	registered, err := h.manager.Register(ctx, &auth)
	if err != nil || registered == nil || strings.TrimSpace(registered.ID) == "" {
		writeError(c, http.StatusInternalServerError, "import OAuth credential failed")
		return
	}
	c.JSON(http.StatusCreated, importedOAuthCredentialResponse{ID: registered.ID, Provider: registered.Provider, Email: email})
}

func decodeOAuthCredentialRequest(c *gin.Context) (coreauth.Auth, error) {
	contentType := strings.TrimSpace(c.GetHeader("Content-Type"))
	mediaType := ""
	if contentType != "" {
		parsed, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return coreauth.Auth{}, err
		}
		mediaType = parsed
	}

	switch mediaType {
	case "", "application/json":
		reader := http.MaxBytesReader(c.Writer, c.Request.Body, maxOAuthCredentialImportBytes)
		auth, err := decodeOAuthCredential(reader)
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return coreauth.Auth{}, errOAuthCredentialTooLarge
		}
		return auth, err
	case "multipart/form-data":
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOAuthCredentialMultipartBytes)
		reader, err := c.Request.MultipartReader()
		if err != nil {
			return coreauth.Auth{}, err
		}
		payload, err := readOAuthCredentialFile(reader)
		if err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				return coreauth.Auth{}, errOAuthCredentialTooLarge
			}
			return coreauth.Auth{}, err
		}
		return decodeOAuthCredential(bytes.NewReader(payload))
	default:
		return coreauth.Auth{}, errOAuthCredentialUnsupportedMedia
	}
}

func readOAuthCredentialFile(reader *multipart.Reader) ([]byte, error) {
	var payload []byte
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if part.FormName() != "file" {
			_, copyErr := io.Copy(io.Discard, part)
			closeErr := part.Close()
			if copyErr != nil {
				return nil, copyErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			continue
		}
		if payload != nil {
			_ = part.Close()
			return nil, errors.New("multiple OAuth credential files")
		}
		data, readErr := io.ReadAll(io.LimitReader(part, maxOAuthCredentialImportBytes+1))
		closeErr := part.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > maxOAuthCredentialImportBytes {
			return nil, errOAuthCredentialTooLarge
		}
		payload = data
	}
	if payload == nil {
		return nil, errors.New("OAuth credential file is missing")
	}
	return payload, nil
}

func decodeOAuthCredential(reader io.Reader) (coreauth.Auth, error) {
	var auth coreauth.Auth
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&auth); err != nil {
		return coreauth.Auth{}, err
	}
	if err := ensureOnlyOneJSONValue(decoder); err != nil {
		return coreauth.Auth{}, err
	}
	return auth, nil
}

func (h *AdminOAuthCredentialHandler) hasDuplicate(provider, email string) bool {
	for _, existing := range h.manager.List() {
		if existing == nil || existing.AuthKind() != coreauth.AuthKindOAuth || !strings.EqualFold(strings.TrimSpace(existing.Provider), provider) {
			continue
		}
		_, existingEmail := existing.AccountInfo()
		if strings.EqualFold(strings.TrimSpace(existingEmail), email) {
			return true
		}
	}
	return false
}

func ensureOnlyOneJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected JSON value")
	}
	return nil
}
