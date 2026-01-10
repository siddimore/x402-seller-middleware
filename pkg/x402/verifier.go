package x402

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// VerifierConfig holds configuration for payment verification
type VerifierConfig struct {
	// Endpoint is the URL of the payment verification service
	Endpoint string

	// APIKey is the API key for authenticating with the payment service
	APIKey string

	// Timeout is the HTTP client timeout
	Timeout time.Duration
}

// VerificationResponse represents the response from a payment verification service
type VerificationResponse struct {
	Valid     bool   `json:"valid"`
	TokenID   string `json:"token_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	ExpiresAt string `json:"expires_at"`
	Error     string `json:"error,omitempty"`
}

// NewHTTPVerifier creates a payment verifier that validates tokens via HTTP
func NewHTTPVerifier(config VerifierConfig) func(token string) (bool, error) {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}

	return func(token string) (bool, error) {
		req, err := http.NewRequest("GET", config.Endpoint, nil)
		if err != nil {
			return false, err
		}

		req.Header.Set("Authorization", "Bearer "+token)
		if config.APIKey != "" {
			req.Header.Set("X-API-Key", config.APIKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return false, nil
		}

		var verifyResp VerificationResponse
		if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
			return false, err
		}

		return verifyResp.Valid, nil
	}
}

// NewStaticVerifier creates a verifier that checks against a list of valid tokens
// Useful for testing and simple use cases
func NewStaticVerifier(validTokens []string) func(token string) (bool, error) {
	tokenSet := make(map[string]struct{}, len(validTokens))
	for _, t := range validTokens {
		tokenSet[t] = struct{}{}
	}

	return func(token string) (bool, error) {
		_, valid := tokenSet[token]
		return valid, nil
	}
}

// NewJWTVerifier creates a verifier for JWT payment tokens
// This is a placeholder - implement based on your JWT library
func NewJWTVerifier(secret string) func(token string) (bool, error) {
	return func(token string) (bool, error) {
		// TODO: Implement JWT verification
		// Example using golang-jwt/jwt:
		//
		// parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		//     return []byte(secret), nil
		// })
		// if err != nil {
		//     return false, err
		// }
		// return parsedToken.Valid, nil

		return false, errors.New("JWT verification not implemented")
	}
}
