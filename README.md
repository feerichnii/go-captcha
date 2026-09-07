<div align="center">
<img width="140" style="padding-top: 40px; margin: 0;" src="assets/gocaptcha_antibot_logo.png" alt="GoCaptcha AntiBot Edition logo"/>
<h1 style="margin: 0; padding: 0">GoCaptcha · AntiBot Edition</h1>
<p>Hardened behavioral CAPTCHA for Golang</p>
<a href="https://goreportcard.com/report/github.com/feerichnii/go-captcha"><img src="https://goreportcard.com/badge/github.com/feerichnii/go-captcha"/></a>
<a href="https://godoc.org/github.com/feerichnii/go-captcha"><img src="https://godoc.org/github.com/feerichnii/go-captcha?status.svg"/></a>
<a href="https://github.com/feerichnii/go-captcha/blob/master/LICENSE"><img src="https://img.shields.io/badge/License-Apache2.0-green.svg"/></a>
</div>

<br/>

<p align="center">
<b>GoCaptcha · AntiBot Edition</b> is a powerful, modular, and highly customizable behavioral CAPTCHA library for Golang. It provides two interactive CAPTCHA types (<b>Slide</b> and <b>Rotate</b>) and layers a full <b>AntiBot</b> stack on top: server-only answers, cryptographic randomness, image interference, AEAD-encrypted challenges, behavior scoring, rate limiting, and adaptive proof-of-work.
</p>

<p align="center"> ⭐️ If it helps you, please give it a star.</p>

<div align="center"> 
<img src="assets/gocaptcha_antibot_poster.png" alt="GoCaptcha AntiBot Edition poster">
</div>

<br/>
<hr/>
<br/>

## What's new in this build

This edition focuses on making the CAPTCHA hard for automated solvers without changing the ergonomics of the original API. Three things ship on top of upstream GoCaptcha:

- **Safer-by-default answers** — `GetPublicData()` returns everything the browser needs and nothing it shouldn't. The real answer (`GetData()`) never has to leave the server, and can be encrypted at rest (AES-256-GCM) by the `antibot` layer or sealed into an opaque AEAD token via [`v2/base/challenge`](v2/base/challenge).
- **Anti-solver image & RNG hardening** — answer geometry now uses `crypto/rand`, JPEG masters ship with added interference noise, slide tiles get multiple identical-silhouette decoy slots, and rotate masters get rim noise. See [SECURITY.md](SECURITY.md).
- **The `antibot` layer** — a drop-in orchestration package ([`v2/antibot`](v2/antibot)) that manages the challenge lifecycle (crypto ID, TTL, **one-shot geometry**), scores pointer trajectories, multi-bucket rate-limits, IP freeze/epoch, bound PoW/JS, and adaptive friction. Backed by in-memory or Redis storage.
- **Bundled readable backgrounds** — landmark-rich scenes under [`v2/resources/backgrounds`](v2/resources/backgrounds) so humans can align the slide tile by eye while bots still face multiple identical notches.

| Capability            | Upstream | AntiBot Edition |
|-----------------------|:--------:|:---------------:|
| Slide / Rotate | ✅ | ✅ |
| Public vs. secret data split  | –  | ✅ `GetPublicData()` |
| Crypto RNG for answers        | –  | ✅ |
| Image interference / decoys   | –  | ✅ |
| Encrypted answers / AEAD tokens | –  | ✅ AES-256-GCM |
| Atomic one-shot geometry       | –  | ✅ claim + freeze / epoch |
| Client binding + verify limits | –  | ✅ |
| Challenge lifecycle manager    | –  | ✅ `antibot` |
| Trajectory behavior scoring    | –  | ✅ |
| Per-client rate limiting       | –  | ✅ |
| Adaptive proof-of-work         | –  | ✅ |

> Jump straight to the [AntiBot layer](#-antibot-layer) or the full [security notes](SECURITY.md).

<br/>

## Ecosystem

| Project                                                                    | Desc                                                                                                                                                                                                      |
|----------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [document](http://gocaptcha.wencodes.com)                                  | GoCaptcha Documentation                                                                                                                                                                                   |
| [online demo](http://gocaptcha.wencodes.com/demo/)                         | GoCaptcha Online Demo                                                                                                                                                                                     |
| [go-captcha-example](https://github.com/wenlng/go-captcha-example)         | Golang + Web + APP Example                                                                                                                                                                                |
| [go-captcha-assets](https://github.com/wenlng/go-captcha-assets)           | Embedded Resource Assets for Golang                                                                                                                                                                       |
| [go-captcha](https://github.com/feerichnii/go-captcha)                         | Golang CAPTCHA Library                                                                                                                                                                                    |
| [go-captcha-jslib](https://github.com/wenlng/go-captcha-jslib)             | JavaScript CAPTCHA Library                                                                                                                                                                                |
| [go-captcha-vue](https://github.com/wenlng/go-captcha-vue)                 | Vue CAPTCHA Library                                                                                                                                                                                       |
| [go-captcha-react](https://github.com/wenlng/go-captcha-react)             | React CAPTCHA Library                                                                                                                                                                                     |
| [go-captcha-angular](https://github.com/wenlng/go-captcha-angular)         | Angular CAPTCHA Library                                                                                                                                                                                   |
| [go-captcha-svelte](https://github.com/wenlng/go-captcha-svelte)           | Svelte CAPTCHA Library                                                                                                                                                                                    |
| [go-captcha-solid](https://github.com/wenlng/go-captcha-solid)             | Solid CAPTCHA Library                                                                                                                                                                                     |
| [go-captcha-uni](https://github.com/wenlng/go-captcha-uni)                 | UniApp CAPTCHA, compatible with Apps, Mini-Programs, and Fast Apps                                                                                                                                        |
| [go-captcha-service](https://github.com/wenlng/go-captcha-service)         | GoCaptcha Service, supports binary and Docker image deployment, <br/>provides HTTP/gRPC interfaces,<br/> supports standalone and distributed modes (service discovery, load balancing, dynamic configuration) |
| [go-captcha-service-sdk](https://github.com/wenlng/go-captcha-service-sdk) | GoCaptcha Service SDK Toolkit, includes HTTP/gRPC request interfaces,<br/> supports static mode, service discovery, and load balancing.                                                                       |
| ...                                                                        |                                                                                                                                                                                                           |

<br/>

## Core Features

- **Diverse CAPTCHA Types**: Supports Slide and Rotate behavioral CAPTCHAs, suitable for various interaction scenarios.
- **Bot-resistant by design**: Cryptographic answer randomness, image interference/decoys, and a public/secret data split so answers never reach the browser.
- **Full AntiBot orchestration**: Challenge lifecycle, trajectory scoring, rate limiting, and adaptive proof-of-work in a single [`antibot`](v2/antibot) package.
- **Highly Customizable**: Flexible configuration of images, fonts, colors, angles, sizes, etc., through Options and Resources.
- **Advanced Image Processing**: Built-in dynamic image generation and processing, supporting main images, thumbnails, puzzle pieces, and shadow effects.
- **Modular Architecture**: Clear code structure, adhering to Go best practices, making it easy to extend and maintain.
- **High-Performance Design**: Optimized resource management and image generation, suitable for high-concurrency scenarios.
- **Cross-Platform Compatibility**: Generated CAPTCHA images can be seamlessly integrated into web applications, mobile apps, or other systems requiring CAPTCHAs.

<br/>

## CAPTCHA Types

`go-captcha` supports the following CAPTCHA types, each with unique interaction methods, generation logic, and application scenarios:

1. **Slide CAPTCHA**: Users slide a puzzle piece horizontally to the correct notch on the main image.
2. **Rotate CAPTCHA**: Users rotate a thumbnail to align with the main image’s angle.

<br/>

## Install
```shell
$ go get -u github.com/feerichnii/go-captcha/v2@latest
```

## Import Module
```go
package main

// Import modules on demand
import "github.com/feerichnii/go-captcha/v2/${slide|rotate}"

func main(){
   // ...
}
```

<br />

## 🖖 Slide CAPTCHA

The Slide CAPTCHA requires users to slide a puzzle piece horizontally to the correct notch on the main image (fixed Y-axis). Multiple identical-silhouette decoy notches are drawn; only one position matches the tile’s background crop.

### How It Works

1. **Generate Main Image** (`masterImage`): Contains the puzzle piece’s notch and shadow effects, typically in JPEG format.
2. **Generate Tile Image** (`tileImage`): The puzzle piece users need to slide, typically in PNG format.
3. **User Interaction**: Users slide the puzzle piece to the target position (`TileX`, `TileY`), and the frontend captures the final coordinates.
4. **Verification Logic**: The backend compares the user’s slide position with the target position to verify a match.

### Code Example
```go
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"io/ioutil"

	"github.com/feerichnii/go-captcha/v2/base/option"
	"github.com/feerichnii/go-captcha/v2/slide"
	"github.com/feerichnii/go-captcha/v2/base/codec"
)

var slideTileCapt slide.Captcha

func init() {
	builder := slide.NewBuilder()

	// You can use preset material resources：https://github.com/wenlng/go-captcha-assets
	bgImage, err := loadPng("../resources/bg.png")
	if err != nil {
		log.Fatalln(err)
	}

	bgImage1, err := loadPng("../resources/bg1.png")
	if err != nil {
		log.Fatalln(err)
	}

	graphs := getSlideTileGraphArr()

	builder.SetResources(
		slide.WithGraphImages(graphs),
		slide.WithBackgrounds([]image.Image{
			bgImage,
			bgImage1,
		}),
	)

	slideTileCapt = builder.Make()
}

func getSlideTileGraphArr() []*slide.GraphImage {
	tileImage1, err := loadPng("../resources/tile-1.png")
	if err != nil {
		log.Fatalln(err)
	}

	tileShadowImage1, err := loadPng("../resources/tile-shadow-1.png")
	if err != nil {
		log.Fatalln(err)
	}
	tileMaskImage1, err := loadPng("../resources/tile-mask-1.png")
	if err != nil {
		log.Fatalln(err)
	}

	return []*slide.GraphImage{
		{
			OverlayImage: tileImage1,
			ShadowImage:  tileShadowImage1,
			MaskImage:    tileMaskImage1,
		},
	}
}

func main() {
	captData, err := slideTileCapt.Generate()
	if err != nil {
		log.Fatalln(err)
	}

	blockData := captData.GetData()
	if blockData == nil {
		log.Fatalln(">>>>> generate err")
	}

	block, _ := json.Marshal(blockData)
	fmt.Println(">>>>>", string(block))

	var mBase64, tBase64 string
	mBase64, err = captData.GetMasterImage().ToBase64()
	if err != nil {
		fmt.Println(err)
	}
	tBase64, err = captData.GetTileImage().ToBase64()
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(">>>>> ", mBase64)
	fmt.Println(">>>>> ", tBase64)
	
	//err = captData.GetMasterImage().SaveToFile("../resources/master.jpg", option.QualityNone)
	//if err != nil {
	//	fmt.Println(err)
	//}
	//err = captData.GetTileImage().SaveToFile("../resources/thumb.png")
	//if err != nil {
	//	fmt.Println(err)
	//}
}

func loadPng(p string) (image.Image, error) {
	imgBytes, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return codec.DecodeByteToPng(imgBytes)
}
```


### Make Instance
- builder.Make()


### Configuration Options
> slide.NewBuilder(slide.WithXxx(), ...) OR builder.SetOptions(slide.WithXxx(), ...)

| Options                                                        | Desc                                           |
|----------------------------------------------------------------|------------------------------------------------|
| slide.WithImageSize(*option.Size)                              | Set main image size, default 300x220           |
| slide.WithImageAlpha(float32)                                  | Set main image transparency                    |
| slide.WithRangeGraphSize(val option.RangeVal)                  | Set range for random graphic size              |
| slide.WithRangeGraphAnglePos([]option.RangeVal)                | Set range for random graphic angles            |
| slide.WithGenGraphNumber(val int)                              | Number of drop slots on the master (default **3**: identical silhouette, 1 correct position). |
| slide.WithEnableGraphVerticalRandom(val bool)                  | Allow each notch its own Y (default off; keep off for horizontal slider) |


### Set Resources
> builder.SetResources(slide.WithXxx(), ...)

| Options                                       | Desc                       |
|-----------------------------------------------|----------------------------|
| slide.WithBackgrounds([]image.Image)          | Set main image backgrounds |
| slide.WithGraphImages(images []*GraphImage)   | Set puzzle piece graphics  |

### Captcha Data

> captData, err := capt.Generate()

| Method                                   | Desc                                                   |
|------------------------------------------|--------------------------------------------------------|
| GetData() *Block                         | Get verification data (**server-only**, secret answer) |
| GetPublicData() interface{}              | Get client-safe metadata (no answer)                   |
| GetMasterImage() imagedata.JPEGImageData | Get main image                                         |
| GetTileImage() imagedata.PNGImageData    | Get tile image                                         |


### Validate the captcha
> ok := slide.Validate(srcX, srcY, X, Y, paddingValue)

| Params       | Desc                  |
|--------------|-----------------------|
| srcX         | User X-axis           |
| srcY         | User Y-axis           |
| X            | X-axis                |
| Y            | Y-axis                |
| paddingValue | Set the padding value |

<br/>

### Notes

- Puzzle piece image resources (`OverlayImage`, `ShadowImage`, `MaskImage`) must be valid, otherwise `ImageTypeErr`, `ShadowImageTypeErr`, or `MaskImageTypeErr` will be triggered.
- Background images must not be empty, otherwise `EmptyBackgroundImageErr` will be triggered.
- The tile’s Y-coordinate matches the notch row; users move only along X (e.g. with a horizontal slider).
- Target notch X is always within `[0, masterWidth − tileWidth]` so a slider can reach every slot.

<br />


## 🖖 Rotate CAPTCHA

The Rotate CAPTCHA requires users to rotate a thumbnail to align with the main image’s angle, suitable for intuitive interaction scenarios.

### How It Works

1. **Generate Main Image** (`masterImage`): Contains a rotated background image, typically in PNG format.
2. **Generate Thumbnail** (`thumbImage`): Cropped from the main image with circular cropping and transparency effects, typically in PNG format.
3. **User Interaction**: Users rotate the thumbnail to the target angle (`block.Angle`), and the frontend captures the rotation angle.
4. **Verification Logic**: The backend compares the user’s rotation angle with the target angle to verify a match.

### Code Example
```go
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"io/ioutil"

	"github.com/feerichnii/go-captcha/v2/rotate"
	"github.com/feerichnii/go-captcha/v2/base/codec"
)

var rotateCapt rotate.Captcha

func init() {
	builder := rotate.NewBuilder()

	// You can use preset material resources：https://github.com/wenlng/go-captcha-assets
	bgImage, err := loadPng("../resources/bg.png")
	if err != nil {
		log.Fatalln(err)
	}

	bgImage1, err := loadPng("../resources/bg1.png")
	if err != nil {
		log.Fatalln(err)
	}

	builder.SetResources(
		rotate.WithImages([]image.Image{
			bgImage,
			bgImage1,
		}),
	)

	rotateCapt = builder.Make()
}

func main() {
	captData, err := rotateCapt.Generate()
	if err != nil {
		log.Fatalln(err)
	}

	blockData := captData.GetData()
	if blockData == nil {
		log.Fatalln(">>>>> generate err")
	}

	block, _ := json.Marshal(blockData)
	fmt.Println(">>>>>", string(block))

	var mBase64, tBase64 string
	mBase64, err = captData.GetMasterImage().ToBase64()
	if err != nil {
		fmt.Println(err)
	}
	tBase64, err = captData.GetThumbImage().ToBase64()
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(">>>>> ", mBase64)
	fmt.Println(">>>>> ", tBase64)
	
	//err = captData.GetMasterImage().SaveToFile("../resources/master.png")
	//if err != nil {
	//	fmt.Println(err)
	//}
	//err = captData.GetThumbImage().SaveToFile("../resources/thumb.png")
	//if err != nil {
	//	fmt.Println(err)
	//}
}

func loadPng(p string) (image.Image, error) {
	imgBytes, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return codec.DecodeByteToPng(imgBytes)
}
```


### Make Instance
- builder.Make()


### Configuration Options
> rotate.NewBuilder(rotate.WithXxx(), ...) OR builder.SetOptions(rotate.WithXxx(), ...)

| Options                                          | Desc                                     |
|--------------------------------------------------|------------------------------------------|
| rotate.WithImageSquareSize(val int)              | Set main image size, default 220x220     |
| rotate.WithRangeAnglePos(vals []option.RangeVal) | Set range for random verification angles |
| rotate.WithRangeThumbImageSquareSize(val []int)  | Set thumbnail size                       |
| rotate.WithThumbImageAlpha(val float32)          | Set thumbnail transparency               |


### Set Resources
> builder.SetResources(rotate.WithXxx(), ...)

| Options                                    | Desc                       |
|--------------------------------------------|----------------------------|
| rotate.WithImages([]image.Image)           | Set main image backgrounds |

### Captcha Data
> captData, err := capt.Generate()

| Method                                   | Desc                                                   |
|------------------------------------------|--------------------------------------------------------|
| GetData() *Block                         | Get verification data (**server-only**, secret answer) |
| GetPublicData() interface{}              | Get client-safe metadata (no answer)                   |
| GetMasterImage() imagedata.PNGImageData  | Get main image                                         |
| GetThumbImage() imagedata.PNGImageData   | Get thumbnail                                          |

### Validate the captcha
> ok := rotate.Validate(srcAngle, angle, paddingValue)

| Params       | Desc                  |
|--------------|-----------------------|
| srcAngle     | User Angle            |
| angle        | Angle                 |
| paddingValue | Set the padding value |

<br/>

### Notes

- Background images must not be empty, otherwise `EmptyImageErr` will be triggered.
- Ensure background images are valid `image.Image` types, otherwise `ImageTypeErr` will be triggered.
- Thumbnails are automatically cropped with a circular effect; ensure background images have sufficient resolution to avoid blurriness.

<br/>
<hr/>

## 🛡 AntiBot layer

The [`v2/antibot`](v2/antibot) package wraps CAPTCHA generation and verification with a full anti-automation pipeline: answers stay on the server, **one visual challenge = one geometry attempt**, tech failures do not consume the challenge, and abuse identity is primarily exact IPv4 `/32` (freeze + epoch) plus session binding.

```
AntiBot layer
├── Challenge Manager   crypto ID, Redis/Memory, TTL 90s, one-shot geometry (no MaxAttempts)
├── IP epoch + active   single live challenge per /32; bad answer invalidates prefetch
├── Answer storage      AES-256-GCM bound to challenge id — never sent to client
├── Binding + freeze    session + HMAC /32; escalating cooldown 2s→5s→15s→60s→300s
├── Bound PoW / JS      challenge+session-bound PoW; rotating JS workloads
├── Trajectory scoring  intervals, dynamics, PointerEvent meta → risk (not proof)
├── Rate limits         hard: session+/32/global; soft: /24+ASN → risk only
└── Browser client      client/antibot-client.js + optional React/Vue helpers
```

### Quick start
```go
import (
    "encoding/json"
    "os"
    "time"

    "github.com/feerichnii/go-captcha/v2/antibot"
    "github.com/feerichnii/go-captcha/v2/slide"
    "github.com/redis/go-redis/v9"
)

secret := []byte(os.Getenv("CAPTCHA_SECRET")) // >= 32 high-entropy bytes
rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
layer, err := antibot.New(antibot.NewRedisStore(rdb), antibot.Config{
    SecretKey:  secret,
    TTL:        90 * time.Second,
    GeoLockTTL: 5 * time.Second,
})

sess, _, _ := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
signals := antibot.SignalsFromRequestTrusted(r, sess, nil)

answer, _ := json.Marshal(captData.GetData()) // secret; encrypted at rest
iss, err := layer.Issue(ctx, antibot.IssueRequest{
    Kind:      antibot.KindSlide,
    Answer:    answer,
    ClientKey: sess.ClientKey,
    Signals:   signals, // authoritative IP required
})
// Client gets: iss.ID, GetPublicData(), images, iss.PoW?, iss.JSChallenge

res, err := layer.Verify(ctx, antibot.VerifyRequest{
    ID:         iss.ID,
    ClientKey:  sess.ClientKey,
    Signals:    signals,
    Browser:    browserFromJSON, // js_challenge_response when browser required
    Answer:     mustJSON(antibot.SlideSubmit{X: ux, Y: uy}),
    Trajectory: antibot.Trajectory{Points: points, Events: events, PieceDown: pieceDown},
    PoWNonce:   nonce, // required if iss.PoW != nil
})
// Tech fail → challenge kept; wrong geometry → freeze + retry_after_ms; score is a risk signal
```

> Never send `GetData()` / stored `Answer` to the browser.  
> Full decision tree, delays/TTLs, and hard vs soft checks: **[`v2/antibot/README.md`](v2/antibot/README.md)**.

<br/>
<hr/>

## Captcha Image Data
### Object Method Of JPEGImageData

| Method                                                     | Desc |
|------------------------------------------------------------|------|
| Get() image.Image                                          |      |
| ToBytes() ([]byte, error)                                  |      |
| ToBytesWithQuality(imageQuality int) ([]byte, error)       |      |
| ToBase64() (string, error)                                 |      |
| ToBase64Data() (string, error)                             |      |
| ToBase64WithQuality(imageQuality int)  (string, error)     |      |
| ToBase64DataWithQuality(imageQuality int) (string, error)  |      |
| SaveToFile(filepath string, quality int) error             |      |


### Object Method Of PNGImageData

| Method                                    | Desc |
|-------------------------------------------|------|
| Get() image.Image                         |      |
| ToBytes() ([]byte, error)                 |      |
| ToBase64() (string, error)                |      |
| ToBase64Data() (string, error)            |      |
| SaveToFile(filepath string) error         |      |


<br/>

## Security

Bot resistance depends on how you wire the library into your app. The essentials:

1. **Never return `GetData()` to clients** — use `GetPublicData()` in API responses, and hand the answer to `antibot.Issue` (encrypted at rest) or seal it with `challenge.Seal` (AEAD).
2. **Expire and single-use challenges** — short TTLs, delete after first successful verify.
3. **One-shot geometry + rate-limit + IP freeze** — the [`antibot`](v2/antibot) layer does this out of the box (see its README for delays/TTLs).
4. **Use diverse assets** — many backgrounds/graphics make solver training harder.

Full details and the rationale behind every hardening change live in [SECURITY.md](SECURITY.md).

<br/>

## Language Support
- [x] Golang
- [ ] NodeJs
- [ ] Rust
- [ ] Python
- [ ] Java
- [ ] PHP
- [ ] ...

## Web
- [x] JavaScript
- [x] Vue
- [x] React
- [x] Angular
- [x] Svelte
- [x] Solid
- [ ] ...

## App
- [x] UniApp
- [ ] Wx-Applet
- [ ] React Native App
- [ ] Flutter App
- [ ] Android App
- [ ] IOS App
- [ ] ...

## Deployment Service
- [x] Binary Program
- [x] Docker Image
- ...

<br/>

## LICENSE
Go Captcha source code is licensed under the Apache Licence, Version 2.0 [http://www.apache.org/licenses/LICENSE-2.0.html](http://www.apache.org/licenses/LICENSE-2.0.html)
