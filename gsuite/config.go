package gsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/helper/logging"
	"github.com/hashicorp/terraform-plugin-sdk/helper/pathorcontents"
	"github.com/pkg/errors"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	directory "google.golang.org/api/admin/directory/v1"
	groupSettings "google.golang.org/api/groupssettings/v1"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

var defaultOauthScopes = []string{
	directory.AdminDirectoryGroupScope,
	directory.AdminDirectoryUserScope,
	directory.AdminDirectoryUserschemaScope,
}

// Config is the structure used to instantiate the GSuite provider.
type Config struct {
	Credentials string
	// Only users with access to the Admin APIs can access the Admin SDK Directory API,
	// therefore the service account needs to impersonate one of those users to access the Admin SDK Directory API.
	// See https://developers.google.com/admin-sdk/directory/v1/guides/delegation
	ImpersonatedUserEmail string

	CustomerId string

	TimeoutMinutes int

	OauthScopes []string

	UpdateExisting bool

	directory *directory.Service

	groupSettings *groupSettings.Service
}

// loadAndValidate loads the application default credentials from the
// environment and creates a client for communicating with Google APIs.
func (c *Config) loadAndValidate(terraformVersion string) error {
	log.Println("[INFO] Building gsuite client config structure")
	var account accountFile

	oauthScopes := c.OauthScopes

	var client *http.Client
	clientOptions := []option.ClientOption{}

	if c.Credentials != "" {
		if c.ImpersonatedUserEmail == "" {
			return fmt.Errorf("required field missing: impersonated_user_email")
		}

		contents, _, err := pathorcontents.Read(c.Credentials)
		if err != nil {
			return fmt.Errorf("Error loading credentials: %s", err)
		}

		// Assume account_file is a JSON string
		if err := parseJSON(&account, contents); err != nil {
			return fmt.Errorf("Error parsing credentials '%s': %s", contents, err)
		}

		// Get the token for use in our requests
		log.Printf("[INFO] Requesting Google token...")
		log.Printf("[INFO]   -- Email: %s", account.ClientEmail)
		log.Printf("[INFO]   -- Scopes: %s", oauthScopes)
		log.Printf("[INFO]   -- Private Key Length: %d", len(account.PrivateKey))

		conf := jwt.Config{
			Email:      account.ClientEmail,
			PrivateKey: []byte(account.PrivateKey),
			Scopes:     oauthScopes,
			TokenURL:   "https://oauth2.googleapis.com/token",
		}

		conf.Subject = c.ImpersonatedUserEmail

		// Initiate an http.Client. The following GET request will be
		// authorized and authenticated on the behalf of
		// your service account.
		client = conf.Client(context.Background())
	} else if c.ImpersonatedUserEmail != "" {
		tokenSource, err := impersonate.CredentialsTokenSource(context.Background(), impersonate.CredentialsConfig{
			TargetPrincipal: c.ImpersonatedUserEmail,
			Scopes:          oauthScopes,
			Subject:         c.ImpersonatedUserEmail,
		})
		if err != nil {
			return errors.Wrap(err, "failed to create impersonated token source")
		}
		clientOptions = append(clientOptions, option.WithTokenSource(tokenSource))

	} else {
		log.Printf("[INFO] Authenticating using DefaultClient")
		err := error(nil)
		client, err = google.DefaultClient(context.Background(), oauthScopes...)
		if err != nil {
			return errors.Wrap(err, "failed to create client")
		}
	}

	// Use a custom user-agent string. This helps google with analytics and it's
	// just a nice thing to do.
	if client != nil {
		client.Transport = logging.NewTransport("Google", client.Transport)
		clientOptions = append(clientOptions, option.WithHTTPClient(client))

	}

	userAgent := fmt.Sprintf("(%s %s) Terraform/%s",
		runtime.GOOS, runtime.GOARCH, terraformVersion)
	context := context.Background()

	// Create the directory service.
	directorySvc, err := directory.NewService(context, clientOptions...)
	if err != nil {
		return err
	}
	directorySvc.UserAgent = userAgent
	c.directory = directorySvc

	// Create the groupSettings service.
	groupSettingsSvc, err := groupSettings.NewService(context, clientOptions...)
	if err != nil {
		return err
	}
	groupSettingsSvc.UserAgent = userAgent
	c.groupSettings = groupSettingsSvc

	return nil
}

// accountFile represents the structure of the account file JSON file.
type accountFile struct {
	PrivateKeyId string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientId     string `json:"client_id"`
}

func parseJSON(result interface{}, contents string) error {
	r := strings.NewReader(contents)
	dec := json.NewDecoder(r)

	return dec.Decode(result)
}
