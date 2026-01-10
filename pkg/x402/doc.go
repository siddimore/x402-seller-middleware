// Package x402 provides HTTP 402 Payment Required middleware implementation.
//
// The x402 package enables Go web applications to require payment before
// granting access to protected resources, following the HTTP 402 specification.
//
// Basic usage:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/api/resource", myHandler)
//
//	handler := x402.Middleware(mux, x402.Config{
//	    PaymentEndpoint:  "https://pay.example.com",
//	    AcceptedMethods:  []string{"Bearer"},
//	    PricePerRequest:  100,
//	    Currency:         "USD",
//	    ExemptPaths:      []string{"/public"},
//	})
//
//	http.ListenAndServe(":8080", handler)
//
// The middleware will return HTTP 402 Payment Required for any request
// to a non-exempt path that doesn't include a valid payment token.
package x402
