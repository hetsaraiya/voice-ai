package internal_connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v2"
	"google.golang.org/api/option"

	config "github.com/rapidaai/api/web-api/config"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
)

type GoogleConnect struct {
	ExternalConnect
	logger      commons.Logger
	oauthCfg    *config.OAuth2Config
	scope       []string
	redirectUrl string
}

var (
	GOOGLE_AUTHENTICATION_STATE = "google"
	GOOGLE_AUTHENTICATION_SCOPE = []string{"email", "profile"}
	GOOGLE_AUTHENTICATION_URL   = "/auth/signin"

	// GOOGLE_DRIVE_STATE       = "connect/"
	GOOGLE_DRIVE_SCOPE = []string{
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/drive.file",
		"https://www.googleapis.com/auth/drive.metadata.readonly",
	}
	GOOGLE_DRIVE_CONNECT_URL = "/connect-knowledge/google-drive"

	GOOGLE_GMAIL_SCOPE = []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/gmail.compose",
		"https://www.googleapis.com/auth/gmail.send",
	}
	GOOGLE_GMAIL_CONNECT_URL = "/connect-action/gmail"
)

func NewGoogleAuthenticationConnect(cfg *config.WebAppConfig, oauthCfg *config.OAuth2Config, logger commons.Logger, postgres connectors.SQLConnector) GoogleConnect {
	return GoogleConnect{
		ExternalConnect: NewExternalConnect(cfg, logger, postgres),
		redirectUrl:     fmt.Sprintf("%s%s", cfg.BaseUrl(), GOOGLE_AUTHENTICATION_URL),
		scope:           GOOGLE_AUTHENTICATION_SCOPE,
		logger:          logger,
		oauthCfg:        oauthCfg,
	}
}

func NewGoogleDriveConnect(cfg *config.WebAppConfig, oauthCfg *config.OAuth2Config, logger commons.Logger, postgres connectors.SQLConnector) GoogleConnect {
	return GoogleConnect{
		ExternalConnect: NewExternalConnect(cfg, logger, postgres),
		redirectUrl:     fmt.Sprintf("%s%s", cfg.BaseUrl(), GOOGLE_DRIVE_CONNECT_URL),
		scope:           GOOGLE_DRIVE_SCOPE,
		logger:          logger,
		oauthCfg:        oauthCfg,
	}
}

func NewGmailConnect(cfg *config.WebAppConfig, oauthCfg *config.OAuth2Config, logger commons.Logger, postgres connectors.SQLConnector) GoogleConnect {
	return GoogleConnect{
		ExternalConnect: NewExternalConnect(cfg, logger, postgres),
		redirectUrl:     fmt.Sprintf("%s%s", cfg.BaseUrl(), GOOGLE_GMAIL_CONNECT_URL),
		scope:           GOOGLE_GMAIL_SCOPE,
		logger:          logger,
		oauthCfg:        oauthCfg,
	}
}

func (gConnect *GoogleConnect) googleOauthConfig() (*oauth2.Config, error) {
	if gConnect.oauthCfg != nil {
		return &oauth2.Config{
			RedirectURL:  gConnect.redirectUrl,
			ClientID:     gConnect.oauthCfg.GoogleClientId,
			ClientSecret: gConnect.oauthCfg.GoogleClientSecret,
			Scopes:       gConnect.scope,
			Endpoint:     google.Endpoint,
		}, nil
	}
	return nil, fmt.Errorf("oauth2-github is not enabled")
}

/*
*

Google implementation
*/

type GoogleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (gtr *GoogleTokenResponse) Token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  gtr.AccessToken,
		TokenType:    gtr.TokenType,
		RefreshToken: gtr.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(gtr.ExpiresIn) * time.Second),
	}
}

func (gtr *GoogleTokenResponse) Map() map[string]interface{} {
	return map[string]interface{}{
		"accessToken":  gtr.AccessToken,
		"tokenType":    gtr.TokenType,
		"refreshToken": gtr.RefreshToken,
		"expiry":       time.Now().Add(time.Duration(gtr.ExpiresIn) * time.Second),
		"scope":        gtr.Scope,
	}
}

// "google"
func (gConnect *GoogleConnect) AuthCodeURL(state string) (string, error) {
	cfg, err := gConnect.googleOauthConfig()
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state), nil
}

func (gConnect *GoogleConnect) Token(c context.Context, code string) (ExternalConnectToken, error) {
	cfg, err := gConnect.googleOauthConfig()
	if err != nil {
		return nil, fmt.Errorf("oauth2-github is not enabled")
	}
	tokenEndpoint := cfg.Endpoint.TokenURL

	// Prepare the data for the request
	data := map[string]string{
		"code":          code,
		"client_id":     cfg.ClientID,
		"client_secret": cfg.ClientSecret,
		"redirect_uri":  cfg.RedirectURL,
		"grant_type":    "authorization_code",
	}

	// Make the request
	resp, err := gConnect.NewHttpClient().R().
		SetContext(c).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(data).
		Post(tokenEndpoint)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != 200 {
		return nil, errors.New("failed to exchange token")
	}

	// Parse the response
	var tokenResponse GoogleTokenResponse
	if err := json.Unmarshal(resp.Body(), &tokenResponse); err != nil {
		return nil, err
	}
	return &tokenResponse, nil
}
func (gConnect *GoogleConnect) GoogleUserInfo(c context.Context, code string) (*OpenID, error) {
	externalToken, err := gConnect.Token(c, code)
	if err != nil {
		gConnect.logger.Errorf("google authentication exchange failed %v", err)
		return nil, fmt.Errorf("code exchange failed: %s", err.Error())
	}
	token := externalToken.Token()
	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		gConnect.logger.Errorf("unable to get userinfo using the access token %v", err)
		return nil, fmt.Errorf("failed getting user info: %s", err.Error())
	}
	defer response.Body.Close()

	var content OpenID
	err = json.NewDecoder(response.Body).Decode(&content)
	content.Source = "google"
	content.Token = token.AccessToken
	if err != nil {
		gConnect.logger.Errorf("unable to decode the response body of the user info %v", err)
		return nil, fmt.Errorf("failed reading response body: %s", err.Error())
	}
	return &content, nil
}

type GoogleFile struct {
	Id       string `json:"id"`
	Title    string `json:"title"`
	Folder   string `json:"folder"`
	FileSize int64  `json:"fileSize,omitempty,string"`
	MimeType string `json:"mimeType"`
}
type GoogleDriveFiles struct {
	Items         []*GoogleFile `json:"items,omitempty"`
	NextPageToken string        `json:"nextPageToken,omitempty"`
}

// "google"
func (gConnect *GoogleConnect) GoogleDriveFiles(ctx context.Context,
	token *oauth2.Token,
	q *string,
	pageToken *string) (*GoogleDriveFiles, error) {
	cfg, err := gConnect.googleOauthConfig()
	if err != nil {
		return nil, err
	}
	driveClient := cfg.Client(ctx, token)
	googleDrive, err := drive.NewService(ctx, option.WithHTTPClient(driveClient))
	if err != nil {
		gConnect.logger.Errorf("google connect with drive files %+v", err)
		return nil, err
	}

	driveRequest := googleDrive.Files.List()
	if pageToken != nil {
		driveRequest = driveRequest.PageToken(*pageToken)
	}
	if q != nil {
		// Q("mimeType='application/vnd.google-apps.folder'").
		driveRequest = driveRequest.Q(*q)
	}
	driveRequest.MaxResults(20)
	driveRequest = driveRequest.
		Fields("nextPageToken", "items(id,fileSize,mimeType,modifiedDate,originalFilename,title,downloadUrl,fileExtension,parents)")

	fls, err := driveRequest.Do()
	if err != nil {
		gConnect.logger.Errorf("google connect with drive files %+v", err)
		return nil, err
	}

	var gdf GoogleDriveFiles
	for _, fl := range fls.Items {
		gdf.Items = append(gdf.Items, &GoogleFile{
			Id:       fl.Id,
			Title:    fl.Title,
			Folder:   gConnect.GetFolderName(googleDrive, fl.Parents),
			FileSize: fl.FileSize,
			MimeType: fl.MimeType,
		})
	}
	gdf.NextPageToken = fls.NextPageToken
	return &gdf, nil
}

func (gConnect *GoogleConnect) GetFolderName(srv *drive.Service, parents []*drive.ParentReference) string {
	if len(parents) == 0 {
		return "Root"
	}
	parentID := parents[0]
	parent, err := srv.Files.Get(parentID.Id).Fields("title").Do()
	if err != nil {
		return "Unknown"
	}
	return parent.Title
}
