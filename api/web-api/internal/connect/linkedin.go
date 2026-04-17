package internal_connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/linkedin"

	config "github.com/rapidaai/api/web-api/config"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
)

type LinkedinConnect struct {
	ExternalConnect
	oauthCfg    *config.OAuth2Config
	redirectURL string
	scopes      []string
	endpoint    oauth2.Endpoint
	logger      commons.Logger
}

var (
	LINKEDIN_AUTHENTICATION_URL   = "/auth/signin"
	LINKEDIN_AUTHENTICATION_SCOPE = []string{"openid", "profile", "email"}

	LINKEDIN_ACTION_CONNECT = "/action/linkedin"
	LINKEDIN_ACTION_SCOPE   = []string{"openid", "profile", "email"}
)

func NewLinkedinAuthenticationConnect(cfg *config.WebAppConfig, oauthCfg *config.OAuth2Config, logger commons.Logger, postgres connectors.SQLConnector) LinkedinConnect {
	return LinkedinConnect{
		ExternalConnect: NewExternalConnect(cfg, logger, postgres),
		redirectURL:     fmt.Sprintf("%s%s", cfg.BaseUrl(), LINKEDIN_AUTHENTICATION_URL),
		scopes:          LINKEDIN_AUTHENTICATION_SCOPE,
		endpoint: oauth2.Endpoint{
			AuthURL:   "https://www.linkedin.com/oauth/v2/authorization",
			TokenURL:  "https://www.linkedin.com/oauth/v2/accessToken",
			AuthStyle: oauth2.AuthStyleInParams,
		},
		oauthCfg: oauthCfg,
		logger:   logger,
	}
}

func NewLinkedinActionConnect(cfg *config.WebAppConfig, oauthCfg *config.OAuth2Config, logger commons.Logger, postgres connectors.SQLConnector) LinkedinConnect {
	return LinkedinConnect{
		ExternalConnect: NewExternalConnect(cfg, logger, postgres),
		redirectURL:     fmt.Sprintf("%s%s", cfg.BaseUrl(), LINKEDIN_ACTION_CONNECT),
		scopes:          LINKEDIN_ACTION_SCOPE,
		endpoint:        linkedin.Endpoint,
		oauthCfg:        oauthCfg,
		logger:          logger,
	}
}

/**

Linkedin oauth
*/

func (liConnect *LinkedinConnect) linkedinOauthConfig() (*oauth2.Config, error) {
	return &oauth2.Config{
		RedirectURL:  liConnect.redirectURL,
		ClientID:     liConnect.oauthCfg.LinkedinClientId,
		ClientSecret: liConnect.oauthCfg.LinkedinClientSecret,
		Scopes:       liConnect.scopes,
		Endpoint:     liConnect.endpoint,
	}, nil
}

func (liConnect *LinkedinConnect) AuthCodeURL(state string) (string, error) {
	cfg, err := liConnect.linkedinOauthConfig()
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state), nil
}

func (liConnect *LinkedinConnect) LinkedinUserInfo(c context.Context, state string, code string) (*OpenID, error) {
	if state != "linkedin" {
		liConnect.logger.Errorf("illegal oauth request as auth state is not matching %s %s", "linkedin", state)
		return nil, fmt.Errorf("invalid oauth state")
	}

	cfg, err := liConnect.linkedinOauthConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid oauth state")
	}

	token, err := cfg.Exchange(c, code)
	if err != nil {
		liConnect.logger.Errorf("unable to exchange the token from linkedin %v", err)
		return nil, err
	}

	client := cfg.Client(c, token)
	req, err := http.NewRequest("GET", "https://api.linkedin.com/v2/userinfo", nil)
	if err != nil {
		liConnect.logger.Errorf("error while creating request %v", err)
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	response, err := client.Do(req)
	if err != nil {
		liConnect.logger.Errorf("error while getting user from linkedin %v", err)
		return nil, err
	}

	defer response.Body.Close()
	// // {"email":"p_srivastav@outlook.com","email_verified":true,"family_name":"Srivastav","given_name":"Prashant","locale":{"country":"US","language":"en"},"name":"Prashant Srivastav","picture":"https://media.licdn.com/dms/image/C5603AQGslsdJ_ZIoMA/profile-displayphoto-shrink_100_100/0/1659118454695?e=1706745600\u0026v=beta\u0026t=8NmYbyO4c6gd3Y1MQjs4LZ3cmh6tYU9zc9Ghlg3FAQ0","sub":"XyBk2_14Uj"}
	var content map[string]interface{}
	err = json.NewDecoder(response.Body).Decode(&content)
	if err != nil {
		liConnect.logger.Errorf("unable to decode %v", err)
		return nil, err
	}
	email, ok := content["email"].(string)
	if !ok {
		return nil, errors.New("missing or invalid email")
	}

	verified, ok := content["email_verified"].(bool)
	if !ok {
		return nil, errors.New("missing or invalid email_verified")
	}

	name, ok := content["name"].(string)
	if !ok {
		return nil, errors.New("missing or invalid name")
	}

	id, ok := content["sub"].(string)
	if !ok {
		return nil, errors.New("missing or invalid sub")
	}

	return &OpenID{
		Token:    token.AccessToken,
		Source:   "linkedin",
		Email:    email,
		Verified: verified,
		Name:     name,
		Id:       id,
	}, nil
}
