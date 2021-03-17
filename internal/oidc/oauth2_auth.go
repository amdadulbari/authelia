package oidc

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/ory/fosite"

	"github.com/authelia/authelia/internal/authorization"
	"github.com/authelia/authelia/internal/configuration/schema"
	"github.com/authelia/authelia/internal/logging"
	"github.com/authelia/authelia/internal/middlewares"
	"github.com/authelia/authelia/internal/session"
	"github.com/authelia/authelia/internal/utils"
)

// OIDCClaims represents a set of OIDC claims.
type OIDCClaims struct {
	jwt.StandardClaims

	Workflow        string   `json:"workflow"`
	Username        string   `json:"username,omitempty"`
	RequestedScopes []string `json:"requested_scopes,omitempty"`
}

func getOIDCClientConfig(clientID string, configuration schema.OpenIDConnectConfiguration) *schema.OpenIDConnectClientConfiguration {
	for _, c := range configuration.Clients {
		if clientID == c.ID {
			return &c
		}
	}

	return nil
}

// nolint:gocyclo
func authorizeHandler(oauth2 fosite.OAuth2Provider, ctx *middlewares.AutheliaCtx, rw http.ResponseWriter, r *http.Request, post bool) {
	ctx.Logger.Debugf("Hit Authorize POST endpoint")

	ar, err := oauth2.NewAuthorizeRequest(r.Context(), r)
	if err != nil {
		logging.Logger().Errorf("Error occurred in NewAuthorizeRequest: %+v", err)
		oauth2.WriteAuthorizeError(rw, ar, err)

		return
	}

	clientID := ar.GetClient().GetID()

	clientConfig := getOIDCClientConfig(clientID, *ctx.Configuration.IdentityProviders.OIDC)
	if clientConfig == nil {
		err := fmt.Errorf("Unable to find related client configuration with name %s", ar.GetID())
		ctx.Logger.Error(err)
		oauth2.WriteAuthorizeError(rw, ar, err)

		return
	}

	if !post {
		targetURL := ar.GetRedirectURI()
		userSession := ctx.GetSession()

		// Resolve the required level of authorizations to proceed with OIDC
		requiredAuthorizationLevel := ctx.Providers.Authorizer.GetRequiredLevel(authorization.Subject{
			Username: userSession.Username,
			Groups:   userSession.Groups,
			IP:       ctx.RemoteIP(),
		}, authorization.NewObjectRaw(targetURL, []byte("GET")))

		isAuthInsufficient := !authorization.IsAuthLevelSufficient(userSession.AuthenticationLevel, requiredAuthorizationLevel)
		isConsentMissing := len(ar.GetRequestedScopes()) > 0 && (userSession.OIDCWorkflowSession == nil ||
			utils.IsStringSlicesDifferent(ar.GetRequestedScopes(), userSession.OIDCWorkflowSession.GrantedScopes))

		if isAuthInsufficient || isConsentMissing {
			forwardedURI, err := ctx.GetOriginalURL()
			if err != nil {
				ctx.Logger.Errorf("%v", err)
				http.Error(rw, err.Error(), 500)

				return
			}

			ctx.Logger.Debugf("User %s must consent with scopes %s", userSession.Username, strings.Join(ar.GetRequestedScopes(), ", "))
			userSession.OIDCWorkflowSession = new(session.OIDCWorkflowSession)
			userSession.OIDCWorkflowSession.ClientID = clientID
			userSession.OIDCWorkflowSession.RequestedScopes = ar.GetRequestedScopes()
			userSession.OIDCWorkflowSession.AuthURI = forwardedURI.String()
			userSession.OIDCWorkflowSession.TargetURI = targetURL.String()
			userSession.OIDCWorkflowSession.RequiredAuthorizationLevel = requiredAuthorizationLevel

			if err := ctx.SaveSession(userSession); err != nil {
				ctx.Logger.Errorf("%v", err)
				http.Error(rw, err.Error(), 500)

				return
			}

			uri, err := ctx.ForwardedProtoHost()
			if err != nil {
				ctx.Logger.Errorf("%v", err)
				http.Error(rw, err.Error(), 500)

				return
			}

			if isConsentMissing {
				http.Redirect(rw, r, fmt.Sprintf("%s/consent", uri), 302)
			} else {
				http.Redirect(rw, r, fmt.Sprintf("%s?workflow=openid", uri), 302)
			}

			return
		}
	}

	scopes := ar.GetRequestedScopes()

	// We grant the requested scopes at this stage.
	for _, scope := range scopes {
		ar.GrantScope(scope)
	}

	audience := ar.GetRequestedAudience()

	for _, a := range audience {
		ar.GrantAudience(a)
	}

	userSession := ctx.GetSession()

	userSession.OIDCWorkflowSession = nil
	if err := ctx.SaveSession(userSession); err != nil {
		ctx.Logger.Errorf("%v", err)
		http.Error(rw, err.Error(), 500)

		return
	}

	// Now that the user is authorized, we set up a session:
	oauthSession := newSession(ctx, scopes, audience)

	// Now we need to get a response. This is the place where the AuthorizeEndpointHandlers kick in and start processing the request.
	// NewAuthorizeResponse is capable of running multiple response type handlers which in turn enables this library
	// to support open id connect.
	response, err := oauth2.NewAuthorizeResponse(ctx, ar, oauthSession)

	// Catch any errors, e.g.:
	// * unknown client
	// * invalid redirect
	// * ...
	if err != nil {
		log.Printf("Error occurred in NewAuthorizeResponse: %+v", err)
		oauth2.WriteAuthorizeError(rw, ar, err)

		return
	}

	// TODO: Record Authorized Responses.

	oauth2.WriteAuthorizeResponse(rw, ar, response)
}

// AuthorizeEndpointPost handles POST requests to the authorize endpoint.
func AuthorizeEndpointPost(oauth2 fosite.OAuth2Provider) middlewares.AutheliaHandlerFunc {
	return func(ctx *middlewares.AutheliaCtx, rw http.ResponseWriter, r *http.Request) {
		ctx.Logger.Debugf("Hit Authorize POST endpoint")

		authorizeHandler(oauth2, ctx, rw, r, true)
	}
}

// AuthorizeEndpointGet handles GET requests to the authorize endpoint.
func AuthorizeEndpointGet(oauth2 fosite.OAuth2Provider) middlewares.AutheliaHandlerFunc {
	return func(ctx *middlewares.AutheliaCtx, rw http.ResponseWriter, r *http.Request) {
		ctx.Logger.Debugf("Hit Authorize GET endpoint")

		authorizeHandler(oauth2, ctx, rw, r, false)
	}
}
