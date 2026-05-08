package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// postJSON marshals body as JSON, POSTs to url with optional Bearer token,
// decodes the response into dest, and returns the HTTP status code.
func postJSON(url, token string, body interface{}, dest interface{}) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(dest) //nolint:errcheck
	return resp.StatusCode, nil
}

// getJSON GETs url with optional Bearer token and decodes the response into dest.
func getJSON(url, token string, dest interface{}) (int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(dest) //nolint:errcheck
	return resp.StatusCode, nil
}

// putJSON marshals body as JSON, PUTs to url with optional Bearer token,
// and decodes the response into dest.
func putJSON(url, token string, body interface{}, dest interface{}) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(dest) //nolint:errcheck
	return resp.StatusCode, nil
}
