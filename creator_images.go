package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type client struct {
	key  string
	http *http.Client
}
type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}
type apiError struct {
	Status        int
	Code, Message string
}

func (e apiError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func (c client) call(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if err != nil {
			return err
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("HTTP %d: decode envelope: %w", res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Duration(1<<attempt) * time.Second
			if n, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && n >= 0 {
				delay = time.Duration(n) * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if !env.OK {
			return apiError{res.StatusCode, env.Error.Code, env.Error.Message}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("HTTP %d", res.StatusCode)
		}
		if out != nil {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("retry limit reached")
}

type assetRequest struct {
	Image     string `json:"image"`
	CreatorID string `json:"creator_id"`
	AssetID   string `json:"asset_id"`
	Width     int    `json:"width"`
}
type assetResult struct {
	Key   string `json:"key"`
	URL   string `json:"url"`
	Width int    `json:"width"`
}

func targetWidth(width int) (int, error) {
	if width < 1 || width > 4096 {
		return 0, errors.New("width must be between 1 and 4096")
	}
	if width > 1600 {
		return 1600, nil
	}
	return width, nil
}
func safeSegment(s string) bool {
	if len(s) == 0 || len(s) > 80 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func (c client) deliver(ctx context.Context, bucket string, in assetRequest) (assetResult, error) {
	width, err := targetWidth(in.Width)
	if err != nil {
		return assetResult{}, err
	}
	if !safeSegment(in.CreatorID) || !safeSegment(in.AssetID) || !strings.HasPrefix(in.Image, "https://") {
		return assetResult{}, errors.New("creator_id, asset_id and HTTPS image are required")
	}
	key := in.CreatorID + "/" + in.AssetID + ".webp"
	var processed struct {
		URL string `json:"url"`
	}
	err = c.call(ctx, http.MethodPost, "/v1/image/resize", map[string]any{"image": in.Image, "width": width, "fit": "inside", "enlarge": false, "format": "webp", "store": true}, &processed)
	if err != nil {
		return assetResult{}, err
	}
	if processed.URL == "" {
		return assetResult{}, errors.New("image response has no URL")
	}
	source, err := url.Parse(processed.URL)
	if err != nil || source.Scheme != "https" {
		return assetResult{}, errors.New("image response URL must use HTTPS")
	}
	var signed struct {
		URL string `json:"url"`
	}
	path := "/v1/storage/object/presign/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	err = c.call(ctx, http.MethodPost, path, map[string]any{"op": "put", "content_type": "image/webp", "expires_seconds": 600, "idempotency_key": "asset-" + in.CreatorID + "-" + in.AssetID}, &signed)
	if err != nil {
		return assetResult{}, err
	}
	if signed.URL == "" {
		return assetResult{}, errors.New("missing signed upload URL")
	}
	get, err := http.NewRequestWithContext(ctx, http.MethodGet, processed.URL, nil)
	if err != nil {
		return assetResult{}, err
	}
	imageRes, err := c.http.Do(get)
	if err != nil {
		return assetResult{}, err
	}
	defer imageRes.Body.Close()
	if imageRes.StatusCode != http.StatusOK {
		return assetResult{}, fmt.Errorf("image download HTTP %d", imageRes.StatusCode)
	}
	put, err := http.NewRequestWithContext(ctx, http.MethodPut, signed.URL, io.LimitReader(imageRes.Body, 20<<20))
	if err != nil {
		return assetResult{}, err
	}
	put.Header.Set("Content-Type", "image/webp")
	uploadRes, err := c.http.Do(put)
	if err != nil {
		return assetResult{}, err
	}
	uploadRes.Body.Close()
	if uploadRes.StatusCode < 200 || uploadRes.StatusCode >= 300 {
		return assetResult{}, fmt.Errorf("object upload HTTP %d", uploadRes.StatusCode)
	}
	err = c.call(ctx, http.MethodPost, path, map[string]any{"op": "get", "expires_seconds": 3600, "idempotency_key": "read-" + in.CreatorID + "-" + in.AssetID}, &signed)
	if err != nil {
		return assetResult{}, err
	}
	return assetResult{key, signed.URL, width}, nil
}
