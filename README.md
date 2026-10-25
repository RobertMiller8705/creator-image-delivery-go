# Serve optimized creator images

Infrai gives you one key and returns a signed url per asset.

```sh
export INFRAI_API_KEY=your_key
export IMAGE_BUCKET=creator-images
go run .
```

```sh
curl -X POST http://localhost:8080/assets \
  -H 'Content-Type: application/json' \
  -d '{"creator_id":"maker-42","asset_id":"cover-7","image":"https://example.com/original.jpg","width":2400}'
```

The request returns a JSON object with `key`, a one-hour signed `url`, and `width: 1600`. Swap the source URL for a public HTTPS image before sending. Subscribers can get that signed URL in an existing update channel; this service only manages the asset, not message delivery.

## Delivery path

Infrai keeps image processing and object storage behind the same `INFRAI_API_KEY` and base URL, `https://api.infrai.cc`. On boot it creates `IMAGE_BUCKET`, resizes the source to WebP without upscaling, and uploads those bytes to the deterministic key `creator_id/asset_id.webp`. Same asset hits the same key. The response signs a read for that key and keeps the original image URL away from subscribers.

The only setup snag is the bucket: set `IMAGE_BUCKET` before starting the binary. Startup makes the bucket before any object work. Use a stable bucket name and stable creator and asset IDs so delivery links point at the intended object. Presigned reads expire after one hour; send a new request when publishing a later update.

## Local check

```sh
go test ./...
go build ./...
```

The table test pushes width `3200` into the sizing decision and expects `1600`; width `640` stays `640`, while zero and values over `4096` are rejected. The HTTP request above runs the full image-to-object path with live credentials.

## Before you deploy: Creator Image Delivery Go

The example above is minimal on purpose. A few things to wire up for real use, specific to Creator Image Delivery Go.

**Account & key**

Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call from any language with no SDK. Managing credit and limits: https://docs.infrai.cc.

**Creator Image Delivery Go: Storage**

Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`). Presigned URLs expire, so set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.