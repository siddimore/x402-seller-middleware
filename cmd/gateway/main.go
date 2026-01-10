// X402 Payment Gateway - A reverse proxy that protects any backend with HTTP 402
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/siddimore/x402-seller-middleware/pkg/x402"
)

func main() {
	// Configuration flags
	listenAddr := flag.String("listen", ":8402", "Gateway listen address")
	backendURL := flag.String("backend", "", "Backend URL to proxy to (e.g., http://localhost:3000)")
	paymentEndpoint := flag.String("payment-url", "", "Payment verification endpoint")
	price := flag.Int64("price", 100, "Price per request in smallest currency unit")
	currency := flag.String("currency", "USD", "Currency code")
	exemptPaths := flag.String("exempt", "/health,/favicon.ico", "Comma-separated exempt paths")
	
	flag.Parse()

	// Allow environment variable overrides
	if env := os.Getenv("X402_BACKEND_URL"); env != "" {
		*backendURL = env
	}
	if env := os.Getenv("X402_PAYMENT_ENDPOINT"); env != "" {
		*paymentEndpoint = env
	}
	if env := os.Getenv("X402_LISTEN_ADDR"); env != "" {
		*listenAddr = env
	}

	if *backendURL == "" {
		log.Fatal("Backend URL is required. Use -backend flag or X402_BACKEND_URL env var")
	}

	// Parse backend URL
	target, err := url.Parse(*backendURL)
	if err != nil {
		log.Fatalf("Invalid backend URL: %v", err)
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	// Custom director to preserve original host header option
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Origin-Host", target.Host)
	}

	// Configure X402 middleware
	config := x402.Config{
		PaymentEndpoint: *paymentEndpoint,
		AcceptedMethods: []string{"Bearer", "Token", "X402"},
		PricePerRequest: *price,
		Currency:        *currency,
		ExemptPaths:     strings.Split(*exemptPaths, ","),
	}

	// Wrap proxy with X402 payment middleware
	handler := x402.Middleware(proxy, config)

	log.Printf("🚀 X402 Payment Gateway starting on %s", *listenAddr)
	log.Printf("🔗 Proxying to: %s", *backendURL)
	log.Printf("💰 Price: %d %s per request", *price, *currency)
	log.Printf("🔓 Exempt paths: %s", *exemptPaths)
	
	log.Fatal(http.ListenAndServe(*listenAddr, handler))
}
