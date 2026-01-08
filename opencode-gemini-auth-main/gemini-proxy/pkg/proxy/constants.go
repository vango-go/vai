package proxy

const (
	CloudCodeAssistEndpoint = "https://cloudcode-pa.googleapis.com"
)

var SpoofHeaders = map[string]string{
	"User-Agent":        "google-api-nodejs-client/9.15.1",
	"X-Goog-Api-Client": "gl-node/22.17.0",
	"Client-Metadata":   "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI",
}

var ModelFallbacks = map[string]string{
	"gemini-2.5-flash-image": "gemini-2.5-flash",
}
