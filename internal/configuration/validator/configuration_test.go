package validator

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/internal/configuration/schema"
)

func newDefaultConfig() schema.Configuration {
	config := schema.Configuration{}
	config.Host = "127.0.0.1"
	config.Port = 9090
	config.LogLevel = "info"
	config.LogFormat = "text"
	config.JWTSecret = testJWTSecret
	config.AuthenticationBackend.File = new(schema.FileAuthenticationBackendConfiguration)
	config.AuthenticationBackend.File.Path = "/a/path"
	config.Session = schema.SessionConfiguration{
		Domain: "example.com",
		Name:   "authelia_session",
		Secret: "secret",
	}
	config.Storage.Local = &schema.LocalStorageConfiguration{
		Path: "abc",
	}
	config.Notifier = &schema.NotifierConfiguration{
		FileSystem: &schema.FileSystemNotifierConfiguration{
			Filename: "/tmp/file",
		},
	}

	return config
}

func TestShouldNotUpdateConfig(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()

	ValidateConfiguration(&config, validator)

	require.Len(t, validator.Errors(), 0)
	assert.Equal(t, 9090, config.Port)
	assert.Equal(t, "info", config.LogLevel)
}

func TestShouldValidateAndUpdatePort(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.Port = 0

	ValidateConfiguration(&config, validator)

	require.Len(t, validator.Errors(), 0)
	assert.Equal(t, 9091, config.Port)
}

func TestShouldValidateAndUpdateHost(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.Host = ""

	ValidateConfiguration(&config, validator)

	require.Len(t, validator.Errors(), 0)
	assert.Equal(t, "0.0.0.0", config.Host)
}

func TestShouldValidateAndUpdateLogsLevel(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.LogLevel = ""

	ValidateConfiguration(&config, validator)

	require.Len(t, validator.Errors(), 0)
	assert.Equal(t, "info", config.LogLevel)
}

func TestShouldEnsureNotifierConfigIsProvided(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 0)

	config.Notifier = nil

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 1)
	assert.EqualError(t, validator.Errors()[0], "A notifier configuration must be provided")
}

func TestShouldAddDefaultAccessControl(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 0)
	assert.NotNil(t, config.AccessControl)
	assert.Equal(t, "deny", config.AccessControl.DefaultPolicy)
}

func TestShouldRaiseErrorWhenTLSCertWithoutKeyIsProvided(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.TLSCert = testTLSCert

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 1)
	assert.EqualError(t, validator.Errors()[0], "No TLS key provided, please check the \"tls_key\" which has been configured")
}

func TestShouldRaiseErrorWhenTLSKeyWithoutCertIsProvided(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.TLSKey = testTLSKey

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 1)
	assert.EqualError(t, validator.Errors()[0], "No TLS certificate provided, please check the \"tls_cert\" which has been configured")
}

func TestShouldNotRaiseErrorWhenBothTLSCertificateAndKeyAreProvided(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.TLSCert = testTLSCert
	config.TLSKey = testTLSKey

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 0)
}

func TestShouldRaiseErrorWithUndefinedJWTSecretKey(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.JWTSecret = ""

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 1)
	assert.EqualError(t, validator.Errors()[0], "Provide a JWT secret using \"jwt_secret\" key")
}

func TestShouldRaiseErrorWithBadDefaultRedirectionURL(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.DefaultRedirectionURL = "abc"

	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 1)
	assert.EqualError(t, validator.Errors()[0], "Unable to parse default redirection url")
}

func TestShouldNotOverrideCertificatesDirectoryAndShouldPassWhenBlank(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	ValidateConfiguration(&config, validator)
	require.Len(t, validator.Errors(), 0)

	require.Equal(t, "", config.CertificatesDirectory)
}

func TestShouldRaiseErrorOnInvalidCertificatesDirectory(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.CertificatesDirectory = "not-a-real-file.go"

	ValidateConfiguration(&config, validator)

	require.Len(t, validator.Errors(), 1)

	if runtime.GOOS == "windows" {
		assert.EqualError(t, validator.Errors()[0], "Error checking certificate directory: CreateFile not-a-real-file.go: The system cannot find the file specified.")
	} else {
		assert.EqualError(t, validator.Errors()[0], "Error checking certificate directory: stat not-a-real-file.go: no such file or directory")
	}

	validator = schema.NewStructValidator()
	config.CertificatesDirectory = "const.go"
	ValidateConfiguration(&config, validator)

	require.Len(t, validator.Errors(), 1)
	assert.EqualError(t, validator.Errors()[0], "The path const.go specified for certificate_directory is not a directory")
}

func TestShouldNotRaiseErrorOnValidCertificatesDirectory(t *testing.T) {
	validator := schema.NewStructValidator()
	config := newDefaultConfig()
	config.CertificatesDirectory = "../../suites/common/ssl"

	ValidateConfiguration(&config, validator)

	require.Len(t, validator.Errors(), 0)
}
