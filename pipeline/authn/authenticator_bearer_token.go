package authn

import (
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"

	"github.com/ory/go-convenience/stringsx"
	"github.com/ory/oathkeeper/x/header"

	"github.com/ory/oathkeeper/driver/configuration"
	"github.com/ory/oathkeeper/helper"
	"github.com/ory/oathkeeper/pipeline"
)

func init() {
	gjson.AddModifier("this", func(json, arg string) string {
		return json
	})
}

type AuthenticatorBearerTokenFilter struct {
}

type AuthenticatorBearerTokenConfiguration struct {
	CheckSessionURL     string                      `json:"check_session_url"`
	BearerTokenLocation *helper.BearerTokenLocation `json:"token_from"`
	PreserveQuery       bool                        `json:"preserve_query"`
	PreservePath        bool                        `json:"preserve_path"`
	PreserveHost        bool                        `json:"preserve_host"`
	ExtraFrom           string                      `json:"extra_from"`
	SubjectFrom         string                      `json:"subject_from"`
	ForwardHTTPHeaders  []string                    `json:"forward_http_headers"`
	SetHeaders          map[string]string           `json:"additional_headers"`
	ForceMethod         string                      `json:"force_method"`
}

func (a *AuthenticatorBearerTokenConfiguration) GetCheckSessionURL() string {
	return a.CheckSessionURL
}

func (a *AuthenticatorBearerTokenConfiguration) GetPreserveQuery() bool {
	return a.PreserveQuery
}

func (a *AuthenticatorBearerTokenConfiguration) GetPreservePath() bool {
	return a.PreservePath
}

func (a *AuthenticatorBearerTokenConfiguration) GetPreserveHost() bool {
	return a.PreserveHost
}

func (a *AuthenticatorBearerTokenConfiguration) GetForwardHTTPHeaders() []string {
	return a.ForwardHTTPHeaders
}

func (a *AuthenticatorBearerTokenConfiguration) GetSetHeaders() map[string]string {
	return a.SetHeaders
}

func (a *AuthenticatorBearerTokenConfiguration) GetForceMethod() string {
	return a.ForceMethod
}

type AuthenticatorBearerToken struct {
	c configuration.Provider
}

var _ AuthenticatorForwardConfig = new(AuthenticatorBearerTokenConfiguration)

func NewAuthenticatorBearerToken(c configuration.Provider) *AuthenticatorBearerToken {
	return &AuthenticatorBearerToken{
		c: c,
	}
}

func (a *AuthenticatorBearerToken) GetID() string {
	return "bearer_token"
}

func (a *AuthenticatorBearerToken) Validate(config json.RawMessage) error {
	if !a.c.AuthenticatorIsEnabled(a.GetID()) {
		return NewErrAuthenticatorNotEnabled(a)
	}

	_, err := a.Config(config)
	return err
}

func (a *AuthenticatorBearerToken) Config(config json.RawMessage) (*AuthenticatorBearerTokenConfiguration, error) {
	var c AuthenticatorBearerTokenConfiguration
	if err := a.c.AuthenticatorConfig(a.GetID(), config, &c); err != nil {
		return nil, NewErrAuthenticatorMisconfigured(a, err)
	}

	if len(c.ExtraFrom) == 0 {
		c.ExtraFrom = "extra"
	}

	if len(c.SubjectFrom) == 0 {
		c.SubjectFrom = "sub"
	}

	// Add Authorization and Cookie headers for backward compatibility
	c.ForwardHTTPHeaders = append(c.ForwardHTTPHeaders, []string{header.Authorization}...)

	return &c, nil
}

func (a *AuthenticatorBearerToken) Authenticate(r *http.Request, session *AuthenticationSession, config json.RawMessage, _ pipeline.Rule) error {
	cf, err := a.Config(config)
	if err != nil {
		return err
	}

	token := helper.BearerTokenFromRequest(r, cf.BearerTokenLocation)
	if token == "" {
		return errors.WithStack(ErrAuthenticatorNotResponsible)
	}

	body, err := forwardRequestToSessionStore(r, cf)
	if err != nil {
		return err
	}

	var (
		subject string
		extra   map[string]interface{}

		subjectRaw = []byte(stringsx.Coalesce(gjson.GetBytes(body, cf.SubjectFrom).Raw, "null"))
		extraRaw   = []byte(stringsx.Coalesce(gjson.GetBytes(body, cf.ExtraFrom).Raw, "null"))
	)

	if err = json.Unmarshal(subjectRaw, &subject); err != nil {
		return helper.ErrForbidden.WithReasonf("The configured subject_from GJSON path returned an error on JSON output: %s", err.Error()).WithDebugf("GJSON path: %s\nBody: %s\nResult: %s", cf.SubjectFrom, body, subjectRaw).WithTrace(err)
	}

	if err = json.Unmarshal(extraRaw, &extra); err != nil {
		return helper.ErrForbidden.WithReasonf("The configured extra_from GJSON path returned an error on JSON output: %s", err.Error()).WithDebugf("GJSON path: %s\nBody: %s\nResult: %s", cf.ExtraFrom, body, extraRaw).WithTrace(err)
	}

	session.Subject = subject
	session.Extra = extra
	return nil
}
