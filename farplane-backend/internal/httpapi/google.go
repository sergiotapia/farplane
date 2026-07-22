package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

const (
	oauthStateTTL        = 10 * time.Minute
	googleUserInfoURL    = "https://openidconnect.googleapis.com/v1/userinfo"
	oauthNonceCookie     = "farplane_oauth_nonce"
	oauthNonceCookiePath = "/api/v1/auth/google"
)

type googleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func (a *api) googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     a.cfg.GoogleClientID,
		ClientSecret: a.cfg.GoogleClientSecret,
		RedirectURL:  a.cfg.GoogleRedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

func (a *api) handleGoogleStart(c *gin.Context) {
	if !a.cfg.GoogleOAuthConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "google oauth is not configured",
		})
		return
	}
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return
	}

	intent := trimNonEmpty(c.Query("intent"))
	if intent == "" {
		intent = auth.OAuthIntentLogin
	}
	orgName := trimNonEmpty(c.Query("organization_name"))

	switch intent {
	case auth.OAuthIntentSetup:
		if orgName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "organization_name is required for setup"})
			return
		}
		if utf8.RuneCountInString(orgName) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "organization_name is too long"})
			return
		}
		if !a.setupTokenOK(c, c.Query("setup_token")) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid setup token"})
			return
		}
		needs, err := a.store.NeedsSetup(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read setup status"})
			return
		}
		if !needs {
			c.JSON(http.StatusConflict, gin.H{"error": "setup already completed"})
			return
		}
	case auth.OAuthIntentLogin:
		needs, err := a.store.NeedsSetup(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read setup status"})
			return
		}
		if needs {
			c.Redirect(http.StatusFound, a.cfg.AppBaseURL+"/setup")
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "intent must be setup or login"})
		return
	}

	nonce, err := auth.NewOAuthNonce()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start oauth"})
		return
	}
	state, err := auth.SignOAuthState(a.cfg.SessionSecret, auth.OAuthState{
		Intent:           intent,
		OrganizationName: orgName,
		Nonce:            nonce,
		ExpiresAtUnix:    time.Now().UTC().Add(oauthStateTTL).Unix(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start oauth"})
		return
	}

	a.setOAuthNonceCookie(c, nonce)
	authURL := a.googleOAuthConfig().AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.SetAuthURLParam("prompt", "select_account"))
	c.Redirect(http.StatusFound, authURL)
}

func (a *api) handleGoogleCallback(c *gin.Context) {
	if !a.cfg.GoogleOAuthConfigured() {
		a.redirectOAuthError(c, "google_oauth_not_configured")
		return
	}
	if a.store == nil {
		a.redirectOAuthError(c, "database_unavailable")
		return
	}

	if errMsg := c.Query("error"); errMsg != "" {
		a.clearOAuthNonceCookie(c)
		a.redirectOAuthError(c, "google_denied")
		return
	}

	state, err := auth.ParseOAuthState(a.cfg.SessionSecret, c.Query("state"), time.Now().UTC())
	if err != nil {
		a.clearOAuthNonceCookie(c)
		a.redirectOAuthError(c, "invalid_state")
		return
	}
	cookieNonce, err := c.Cookie(oauthNonceCookie)
	if err != nil || cookieNonce == "" || subtle.ConstantTimeCompare([]byte(cookieNonce), []byte(state.Nonce)) != 1 {
		a.clearOAuthNonceCookie(c)
		a.redirectOAuthError(c, "invalid_state")
		return
	}
	a.clearOAuthNonceCookie(c)

	code := c.Query("code")
	if code == "" {
		a.redirectOAuthError(c, "missing_code")
		return
	}

	ctx := c.Request.Context()
	tok, err := a.googleOAuthConfig().Exchange(ctx, code)
	if err != nil {
		a.redirectOAuthError(c, "token_exchange_failed")
		return
	}
	info, err := fetchGoogleUserInfo(ctx, tok.AccessToken)
	if err != nil {
		a.redirectOAuthError(c, "userinfo_failed")
		return
	}
	if info.Sub == "" || info.Email == "" || !info.EmailVerified {
		a.redirectOAuthError(c, "incomplete_profile")
		return
	}
	email, emailOK := normalizeEmail(info.Email)
	if !emailOK {
		a.redirectOAuthError(c, "incomplete_profile")
		return
	}

	displayName := strings.TrimSpace(info.Name)
	if displayName == "" {
		displayName = email
	}
	if utf8.RuneCountInString(displayName) > 200 {
		displayName = string([]rune(displayName)[:200])
	}
	var avatarURL *string
	if pic := strings.TrimSpace(info.Picture); pic != "" && strings.HasPrefix(strings.ToLower(pic), "https://") {
		avatarURL = &pic
	}

	switch state.Intent {
	case auth.OAuthIntentSetup:
		token, err := auth.NewSessionToken()
		if err != nil {
			a.redirectOAuthError(c, "session_failed")
			return
		}
		expiresAt := time.Now().UTC().Add(a.cfg.SessionTTL)
		_, err = a.store.CompleteGoogleSetup(ctx, store.SetupGoogleInput{
			OrganizationName: state.OrganizationName,
			Email:            email,
			DisplayName:      displayName,
			AvatarURL:        avatarURL,
			ProviderSubject:  info.Sub,
			SessionToken:     token,
			SessionExpiresAt: expiresAt,
		})
		if err != nil {
			if errors.Is(err, store.ErrAlreadySetup) {
				a.redirectOAuthError(c, "setup_already_completed")
				return
			}
			a.redirectOAuthError(c, "setup_failed")
			return
		}
		a.setSessionCookie(c, token, expiresAt)
		c.Redirect(http.StatusFound, a.cfg.AppBaseURL+"/")
		return

	case auth.OAuthIntentLogin:
		userID, err := a.store.GetUserIDByGoogleSubject(ctx, info.Sub)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				a.redirectOAuthError(c, "account_not_found")
				return
			}
			a.redirectOAuthError(c, "login_failed")
			return
		}
		token, expiresAt, err := a.createSessionForUser(c, userID)
		if err != nil {
			a.redirectOAuthError(c, "session_failed")
			return
		}
		a.setSessionCookie(c, token, expiresAt)
		c.Redirect(http.StatusFound, a.cfg.AppBaseURL+"/")
		return

	default:
		a.redirectOAuthError(c, "invalid_intent")
	}
}

func (a *api) redirectOAuthError(c *gin.Context, code string) {
	path := "/login"
	if strings.HasPrefix(code, "setup_") {
		path = "/setup"
	}
	u, err := url.Parse(a.cfg.AppBaseURL + path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": code})
		return
	}
	q := u.Query()
	q.Set("oauth_error", code)
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

func (a *api) setOAuthNonceCookie(c *gin.Context, nonce string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		oauthNonceCookie,
		nonce,
		int(oauthStateTTL.Seconds()),
		oauthNonceCookiePath,
		"",
		a.cfg.SessionCookieSecure,
		true,
	)
}

func (a *api) clearOAuthNonceCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthNonceCookie, "", -1, oauthNonceCookiePath, "", a.cfg.SessionCookieSecure, true)
}

func fetchGoogleUserInfo(ctx context.Context, accessToken string) (googleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return googleUserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return googleUserInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return googleUserInfo{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return googleUserInfo{}, fmt.Errorf("userinfo status %d: %s", resp.StatusCode, string(body))
	}

	var info googleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return googleUserInfo{}, err
	}
	return info, nil
}
