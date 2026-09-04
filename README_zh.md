<div align="center">
<img width="140" style="padding-top: 40px; margin: 0;" src="assets/gocaptcha_antibot_logo.png" alt="GoCaptcha AntiBot Edition logo"/>
<h1 style="margin: 0; padding: 0">GoCaptcha · AntiBot 加固版</h1>
<p>面向 Golang 的加固型行为验证码</p>
<a href="https://goreportcard.com/report/github.com/wenlng/go-captcha"><img src="https://goreportcard.com/badge/github.com/wenlng/go-captcha"/></a>
<a href="https://godoc.org/github.com/wenlng/go-captcha"><img src="https://godoc.org/github.com/wenlng/go-captcha?status.svg"/></a>
<a href="https://github.com/wenlng/go-captcha/blob/v2/LICENSE"><img src="https://img.shields.io/badge/License-Apache2.0-green.svg"/></a>
</div>

<br/>

> [English](README.md) | 中文 
<p align="center">
<b>GoCaptcha · AntiBot 加固版</b> 是一个功能强大、模块化且高度可定制的 Golang 行为式验证码库。它提供全部四种交互式验证码类型（<b>点选</b>、<b>滑动</b>、<b>拖拽</b> 和 <b>旋转</b>），并在其之上叠加了完整的 <b>AntiBot（反自动化）</b> 能力：答案仅保留在服务端、加密随机数、图像干扰、HMAC 密封挑战、行为轨迹评分、限流以及自适应工作量证明（PoW）。
</p>

<p align="center"> ⭐️ 如果能帮助到你，请随手给个 star</p>

<br/>

<div align="center">
<img src="assets/gocaptcha_antibot_poster.png" alt="GoCaptcha AntiBot Edition poster">
</div>

<br/>
<hr/>
<br/>

## 本次构建的新增内容

本加固版在不改变原有 API 使用习惯的前提下，让验证码更难被自动化程序破解。相较上游 GoCaptcha，新增三大能力：

- **默认更安全的答案处理** —— `GetPublicData()` 只返回浏览器所需、且不含答案的元数据；真实答案（`GetData()`）无需离开服务端，也可通过 [`v2/base/challenge`](v2) 密封为不透明的 HMAC 令牌。
- **抗破解的图像与随机数加固** —— 答案坐标改用 `crypto/rand` 生成，JPEG 主图叠加干扰噪点，滑块加入诱饵阴影与边缘抖动，旋转主图加入圆环噪点，点选缩略图默认形变。详见 [SECURITY.md](SECURITY.md)。
- **`antibot` 编排层** —— 开箱即用的编排包（[`v2/antibot`](v2/antibot)），负责挑战生命周期（加密 ID、TTL、单次使用、尝试次数上限）、指针轨迹评分、按客户端限流，并对可疑客户端下发自适应 PoW。支持内存或 Redis 存储。
- **内置高复杂度背景图** —— 在 [`v2/resources/backgrounds`](v2/resources/backgrounds) 内置了一组高熵、细节密集的全新背景图，让主图默认就更难被 OCR/轮廓类自动化程序分割识别。

> 直接跳转到 [AntiBot 反自动化层](#-antibot-反自动化层) 或完整的 [安全说明](SECURITY.md)。

<br/>

## 项目生态

| 名称                                                                         | 描述                                                                                          |
|----------------------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| [document](http://gocaptcha.wencodes.com)                                  | GoCaptcha 文档                                                                                |
| [online demo](http://gocaptcha.wencodes.com/demo/)                         | GoCaptcha 在线演示                                                                              |
| [go-captcha-example](https://github.com/wenlng/go-captcha-example)         | Golang + 前端 + APP实例                                                                         |
| [go-captcha-assets](https://github.com/wenlng/go-captcha-assets)           | Golang 内嵌素材资源                                                                               |
| [go-captcha](https://github.com/wenlng/go-captcha)                         | Golang 验证码                                                                                  |
| [go-captcha-jslib](https://github.com/wenlng/go-captcha-jslib)             | Javascript 验证码                                                                              |
| [go-captcha-vue](https://github.com/wenlng/go-captcha-vue)                 | Vue 验证码                                                                                     |
| [go-captcha-react](https://github.com/wenlng/go-captcha-react)             | React 验证码                                                                                   |
| [go-captcha-angular](https://github.com/wenlng/go-captcha-angular)         | Angular 验证码                                                                                 |
| [go-captcha-svelte](https://github.com/wenlng/go-captcha-svelte)           | Svelte 验证码                                                                                  |
| [go-captcha-solid](https://github.com/wenlng/go-captcha-solid)             | Solid 验证码                                                                                   |
| [go-captcha-uni](https://github.com/wenlng/go-captcha-uni)                 | UniApp 验证码，兼容 App、小程序、快应用等                                                                  |
| [go-captcha-service](https://github.com/wenlng/go-captcha-service)         | GoCaptcha 服务，支持二进制、Docker镜像等方式部署，<br/> 提供 HTTP/GRPC 方式访问接口，<br/>可用单机模式和分布式（服务发现、负载均衡、动态配置等） |
| [go-captcha-service-sdk](https://github.com/wenlng/go-captcha-service-sdk) | GoCaptcha 服务SDK工具包，包含 HTTP/GRPC 请求服务接口，<br/>支持静态模式、服务发现、负载均衡                                |
| ...                                                                        |                                                                                             |

<br/>

## 核心特性

- **多样化验证码类型**：支持点选、滑动、旋转和拖拽四种行为式验证码，适应不同交互场景。
- **默认抗自动化**：答案使用加密随机数生成、图像干扰/诱饵，并对公开数据与秘密答案做拆分，答案永不下发到浏览器。
- **完整 AntiBot 编排**：挑战生命周期、轨迹评分、限流与自适应 PoW，统一封装在 [`antibot`](v2/antibot) 包中。
- **高度可定制化**：通过选项（`Options`）和资源（`Resources`）支持图像、字体、颜色、角度、大小等灵活配置。
- **高级图像处理**：内置动态图像生成和处理功能，支持主图像、缩略图、拼图块和阴影效果的生成。
- **模块化架构**：代码结构清晰，遵循 Go 语言最佳实践，易于扩展和维护。
- **高性能设计**：优化资源管理和图像生成，适合高并发场景。
- **跨平台兼容**：生成的验证码图像可无缝集成到 Web 应用、移动应用或其他需要验证码的系统。

<br/>

## 验证码类型

`go-captcha` 支持以下四种验证码类型，每种类型具有独特的交互方式、生成逻辑和应用场景：
1. **点选验证码（Click）**：用户在主图像中点击指定的点或字符，支持文本模式和图形模式。
2. **滑动验证码（Slide）**：用户将拼图块滑动到主图像中的正确位置，支持基本模式和拖拽模式。
3. **拖拽验证码（DragDrop）**：滑动验证码的变体，允许用户在更大范围内拖动拼图块到目标位置。
4. **旋转验证码（Rotate）**：用户旋转缩略图使其与主图像的角度对齐。

<br/>

## 设置Go代理
- Window
```shell
$ set GO111MODULE=on
$ set GOPROXY=https://goproxy.io,direct

### The Golang 1.13+ can be executed directly
$ go env -w GO111MODULE=on
$ go env -w GOPROXY=https://goproxy.io,direct
```
- Linux or Mac
```shell
$ export GO111MODULE=on
$ export GOPROXY=https://goproxy.io,direct

### or
$ echo "export GO111MODULE=on" >> ~/.profile
$ echo "export GOPROXY=https://goproxy.cn,direct" >> ~/.profile
$ source ~/.profile
```

## 安装
```shell
$ go get -u github.com/wenlng/go-captcha/v2@latest
```

## 按需引入模块
```go
package main

// 按需求引入对应的模块
import "github.com/wenlng/go-captcha/v2/${click|slide|rotate}"

func main(){
   // ....
}
```

<br />

## 🖖 点选验证码（Click）

点选验证码要求用户在主图像中点击指定的点或字符，适合需要快速验证的场景。支持两种模式：

- **文本模式**：显示字符（如字母、数字或中文），用户点击对应字符。
- **图形模式**：显示图形（如图标或形状），用户点击对应图形。

### 工作原理

1. **生成主图像**（`masterImage`）：包含随机分布的点或字符，通常为 JPEG 格式。
2. **生成缩略图**（`thumbImage`）：显示需要点击的目标点或字符，通常为 PNG 格式。
3. **用户交互**：用户点击主图像中的坐标，前端捕获坐标并发送到后端。
4. **验证逻辑**：后端比较用户点击的坐标与目标点（`dots`）的坐标是否匹配。

### 代码示例
```go
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"io/ioutil"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/click"
	"github.com/wenlng/go-captcha/v2/base/codec"
)

var textCapt click.Captcha

func init() {
	builder := click.NewBuilder(
		click.WithRangeLen(option.RangeVal{Min: 4, Max: 6}),
		click.WithRangeVerifyLen(option.RangeVal{Min: 2, Max: 4}),
        // ...
	)

	// 可以使用预置的素材资源：https://github.com/wenlng/go-captcha-assets
	fontN, err := loadFont("../resources/fzshengsksjw_cu.ttf")
	if err != nil {
		log.Fatalln(err)
	}

	bgImage, err := loadPng("../resources/bg.png")
	if err != nil {
		log.Fatalln(err)
	}

	builder.SetResources(
		click.WithChars([]string{"这", "是", "随", "机", "的", "文", "本", "种", "子", "呀"}),
		click.WithFonts([]*truetype.Font{
			fontN,
		}),
		click.WithBackgrounds([]image.Image{
			bgImage,
		}),
	)

	textCapt = builder.Make()
}

func loadPng(p string) (image.Image, error) {
	imgBytes, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return codec.DecodeByteToPng(imgBytes)
}

func loadFont(p string) (*truetype.Font, error) {
	fontBytes, err := ioutil.ReadFile(p)
	if err != nil {
		panic(err)
	}
	return freetype.ParseFont(fontBytes)
}


func main() {
	captData, err := textCapt.Generate()
	if err != nil {
		log.Fatalln(err)
	}

	dotData := captData.GetData()
	if dotData == nil {
		log.Fatalln(">>>>> generate err")
	}

	dots, _ := json.Marshal(dotData)
	fmt.Println(">>>>> ", string(dots))

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
	
	//err = captData.GetMasterImage().SaveToFile("../resources/master.jpg", option.QualityNone)
	//if err != nil {
	//	fmt.Println(err)
	//}
	//err = captData.GetThumbImage().SaveToFile("../resources/thumb.png")
	//if err != nil {
	//	fmt.Println(err)
	//}
}
```

### 创建实例
- builder.Make()  中文文本、字母数字混合点选
- builder.MakeShape()  图形点选

### 配置选项
> click.NewBuilder(click.WithXxx(), ...) 或 builder.SetOptions(click.WithXxx(), ...)

| Options                                    | Desc                                                  |
|--------------------------------------------|-------------------------------------------------------|
| 主图                                         |
| click.WithImageSize(option.Size)           | 设置主图尺寸，默认 300x220                                     |
| click.WithRangeLen(option.RangeVal)        | 设置随机内容长度范围                                            |
| click.WithRangeAnglePos([]option.RangeVal) | 设置随机角度范围                                              |
| click.WithRangeSize(option.RangeVal)       | 设置随机内容大小范围                                            |
| click.WithRangeColors([]string)            | 设置随机颜色                                                |
| click.WithDisplayShadow(bool)              | 设置是否显示阴影                                              |
| click.WithShadowColor(string)              | 设置阴影颜色                                                |
| click.WithShadowPoint(option.Point)        | 设置阴影偏移位置                                              |
| click.WithImageAlpha(float32)              | 设置主图透明度                                               |
| click.WithUseShapeOriginalColor(bool)      | 设置是否使用图形原始颜色，"图形点选"有效                                 |
| 缩略图                                        |
| click.WithThumbImageSize(option.Size)      | 设置缩略尺寸，默认 150x40                                      |
| click.WithRangeVerifyLen(option.RangeVal)  | 设置校验内容的随机长度范围                                         |
| click.WithDisabledRangeVerifyLen(bool)     | 禁用校验内容的随机长度，与主图内容的长度保持一致                              |
| click.WithRangeThumbSize(option.RangeVal)  | 设置随机缩略内容随机大小范围                                        |
| click.WithRangeThumbColors([]string)       | 设置缩略随机颜色范围                                            |
| click.WithRangeThumbBgColors([]string)     | 设置缩略随机背景颜色范围                                          |
| click.WithIsThumbNonDeformAbility(bool)    | 设置缩略图内容不变形，不受背景影响                                     |
| click.WithThumbBgDistort(int)              | 设置缩略图背景扭曲 option.DistortLevel1 至 option.DistortLevel5 |
| click.WithThumbBgCirclesNum(int)           | 设置缩略图绘制小圆点数量                                          |
| click.WithThumbBgSlimLineNum(int)          | 设置缩略图绘制线条数量                                           |


### 设置资源
> builder.SetResources(click.WithXxx(), ...)

| Options                                   | Desc      |
|-------------------------------------------|-----------|
| click.WithChars([]string)                 | 设置文本种子    |
| click.WithShapes(map[string]image.Image)  | 设置图形种子    |
| click.WithFonts([]*truetype.Font)         | 设置字体      |
| click.WithBackgrounds([]image.Image)      | 设置主图背景    |
| click.WithThumbBackgrounds([]image.Image) | 设置缩略图背景   |


### 验证码数据
> captData, err := capt.Generate()

| Method                                   | Desc      |
|------------------------------------------|-----------|
| GetData() map[int]*Dot                   | 获取当前校验的信息 |
| GetMasterImage() imagedata.JPEGImageData | 获取主图      |
| GetThumbImage() imagedata.PNGImageData   | 获取缩略图     |

### 验证码校验
> ok := click.Validate(srcX, srcY, X, Y, width, height, paddingValue)

| Params       | Desc               |
|--------------|--------------------|
| srcX         | 用户交互的 X 值          |
| srcY         | 用户交互的 Y 值          |
| X            | 验证码校验的 X 值         |
| Y            | 验证码校验的 Y 值         |
| width        | 验证码校验的 Width 值     |
| height       | 验证码校验的 Height 值    |
| paddingValue | 控制误差值              |

<br/>

### 注意事项

- 字符集（`chars`）或图形集（`shapes`）的长度必须大于 `rangeLen.Max`，否则会触发 `CharRangeLenErr` 或 `ShapesRangeLenErr`。
- 图形模式需要提供有效的图像资源（`shapeMaps`），否则会触发 `ShapesTypeErr`。
- 背景图像不能为空，否则会触发 `EmptyBackgroundImageErr`。

<br />

## 🖖 滑动/拖拽验证码（Slide/Drag-Drop）

滑动验证码要求用户将拼图块滑动到主图像中的正确位置，支持两种模式：

- **基本模式**：拼图块沿固定 Y 轴滑动，适合简单验证场景。
- **拖拽模式**：拼图块可以在更大范围内自由拖动，适合需要更高交互自由度的场景。

### 工作原理

1. **生成主图像**（`masterImage`）：包含拼图块的缺口和阴影效果，通常为 JPEG 格式。
2. **生成拼图图像**（`tileImage`）：用户需要滑动的拼图块，通常为 PNG 格式。
3. **用户交互**：用户滑动拼图块到目标位置（`TileX`, `TileY`），前端捕获滑动终点坐标。
4. **验证逻辑**：后端比较用户滑动的位置与目标位置是否匹配。

### 代码示例
```go
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"io/ioutil"

	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
	"github.com/wenlng/go-captcha/v2/base/codec"
)

var slideTileCapt slide.Captcha

func init() {
	builder := slide.NewBuilder()

	// 可以使用预置的素材资源：https://github.com/wenlng/go-captcha-assets
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


### 创建实例
- builder.Make() 滑动式
- builder.MakeDragDrop()  区域内拖拽式


### 配置选项
> slide.NewBuilder(slide.WithXxx(), ...) 或 builder.SetOptions(slide.WithXxx(), ...)

| Options                                                        | Desc              |
|----------------------------------------------------------------|-------------------|
| slide.WithImageSize(*option.Size)                              | 设置主图尺寸，默认 300x220 |
| slide.WithImageAlpha(float32)                                  | 设置主图透明度           |
| slide.WithRangeGraphSize(val option.RangeVal)                  | 设置图形随机尺寸范围        |
| slide.WithRangeGraphAnglePos([]option.RangeVal)                | 设置图形随机角度范围        |
| slide.WithGenGraphNumber(val int)                              | 设置图形个数            |
| slide.WithEnableGraphVerticalRandom(val bool)                  | 设置图形水平方向是否随机排序    |
| slide.WithRangeDeadZoneDirections(val []DeadZoneDirectionType) | 设置贴图盲区            |


### 设置资源
> builder.SetResources(slide.WithXxx(), ...)

| Options                                       | Desc     |
|-----------------------------------------------|----------|
| slide.WithBackgrounds([]image.Image)          | 设置主图背景   |
| slide.WithGraphImages(images []*GraphImage)   | 设置贴图的图形  |


### 验证码数据
> captData, err := capt.Generate()

| Method                                   | Desc        |
|------------------------------------------|-------------|
| GetData() *Block                         | 获取当前校验的信息   |
| GetMasterImage() imagedata.JPEGImageData | 获取主图        |
| GetTileImage() imagedata.PNGImageData    | 获取缩略图       |


### 验证码校验
> ok := slide.Validate(srcX, srcY, X, Y, paddingValue)

| Params       | Desc            |
|--------------|-----------------|
| srcX         | 用户交互的 X 值       |
| srcY         | 用户交互的 Y 值       |
| X            | 验证码校验的 X 值      |
| Y            | 验证码校验的 Y 值      |
| paddingValue | 控制误差值           |

<br/>

### 注意事项

- 拼图块的图像资源（`OverlayImage`, `ShadowImage`, `MaskImage`）必须有效，否则会触发 `ImageTypeErr`, `ShadowImageTypeErr` 或 `MaskImageTypeErr`。
- 背景图像不能为空，否则会触发 `EmptyBackgroundImageErr`。
- 基本模式下，拼图块的 Y 坐标固定；拖拽模式下，Y 坐标可根据 `rangeDeadZoneDirections` 随机分布。
- 拖拽模式适合需要更高交互自由度的场景，但可能增加用户操作时间。

<br />


## 🖖  旋转验证码（Rotate）

旋转验证码要求用户旋转缩略图使其与主图像的角度对齐，适合需要直观交互的场景。

### 工作原理

1. **生成主图像**（`masterImage`）：包含旋转后的背景图像，通常为 PNG 格式。
2. **生成缩略图**（`thumbImage`）：从主图像裁剪并应用圆形裁剪和透明度效果，通常为 PNG 格式。
3. **用户交互**：用户旋转缩略图到目标角度（`block.Angle`），前端捕获旋转角度。
4. **验证逻辑**：后端比较用户旋转的角度与目标角度是否匹配。

### 代码示例
```go
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"io/ioutil"

	"github.com/wenlng/go-captcha/v2/rotate"
	"github.com/wenlng/go-captcha/v2/base/codec"
)

var rotateCapt rotate.Captcha

func init() {
	builder := rotate.NewBuilder()

	// 可以使用预置的素材资源：https://github.com/wenlng/go-captcha-assets
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


### 创建实例
- builder.Make() 旋转式


### 配置选项
> rotate.NewBuilder(rotate.WithXxx(), ...) 或 builder.SetOptions(rotate.WithXxx(), ...)

| Options                                          | Desc              |
|--------------------------------------------------|-------------------|
| rotate.WithImageSquareSize(val int)              | 设置主图大小，默认 220x220 |
| rotate.WithRangeAnglePos(vals []option.RangeVal) | 设置校验随机角度范围        |
| rotate.WithRangeThumbImageSquareSize(val []int)  | 设置缩略图大小           |
| rotate.WithThumbImageAlpha(val float32)          | 设置缩略图透明度          |


### 设置资源
> builder.SetResources(rotate.WithXxx(), ...)

| Options                                    | Desc       |
|--------------------------------------------|------------|
| rotate.WithBackgrounds([]image.Image)      | 设置主图图片     |


### 验证码数据
> captData, err := capt.Generate()

| Method                                   | Desc        |
|------------------------------------------|-------------|
| GetData() *Block                         | 获取当前校验的信息   |
| GetMasterImage() imagedata.JPEGImageData | 获取主图        |
| GetTileImage() imagedata.PNGImageData    | 获取缩略图       |


### 验证码校验
> ok := rotate.Validate(srcAngle, angle, paddingValue)

| Params       | Desc  |
|--------------|-------|
| srcAngle     | 用户交互的角度 |
| angle        | 验证码校验的角度 |
| paddingValue | 控制误差值 |

<br/>

### 注意事项

- 背景图像不能为空，否则会触发 `EmptyImageErr`。
- 确保背景图像是有效的 `image.Image` 类型，否则会触发 `ImageTypeErr`。
- 缩略图会自动应用圆形裁剪效果，确保背景图像分辨率足够以避免模糊。

<br/>
<hr/>

## 🛡 AntiBot 反自动化层

[`v2/antibot`](v2/antibot) 包在验证码的生成与校验之外，提供了一整套反自动化流水线：答案始终保留在服务端，可疑客户端则会被施加额外阻力。

```
AntiBot layer
├── 挑战管理器（Challenge Manager）  加密 ID、Redis/内存、TTL 90s、最多 3 次尝试、单次使用
├── 轨迹采集（Trajectory Collector）  x,y,t + 指针事件（客户端 → 服务端）
├── 行为评分（Behavior Scoring）      时长、速度、加速度、时序、纠偏
├── 限流器（Rate Limiter）           按客户端 key 限流
└── 自适应挑战（Adaptive Challenge）  对可疑客户端下发 PoW
```

### 快速开始
```go
import (
    "encoding/json"
    "time"

    "github.com/wenlng/go-captcha/v2/antibot"
    "github.com/wenlng/go-captcha/v2/slide"
    // "github.com/redis/go-redis/v9"
)

store := antibot.NewMemoryStore()
// 或：antibot.NewRedisStore(redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}))

layer := antibot.New(store, antibot.Config{
    TTL:         90 * time.Second,
    MaxAttempts: 3,
})

// slide.Generate() 之后：
answer, _ := json.Marshal(captData.GetData()) // 仅服务端使用
iss, err := layer.Issue(ctx, antibot.IssueRequest{
    Kind:       antibot.KindSlide,
    Answer:     answer,
    ClientKey:  clientIP,
    Suspicious: false,
})
// 客户端获得：iss.ID、captData.GetPublicData()、图像、iss.PoW（可选）

res, err := layer.Verify(ctx, antibot.VerifyRequest{
    ID:         iss.ID,
    Answer:     mustJSON(antibot.SlideSubmit{X: ux, Y: uy}),
    Trajectory: antibot.Trajectory{Points: points, Events: events},
    PoWNonce:   nonce, // 当 iss.PoW != nil 时
})
```

> 切勿将 `GetData()` / 存储的 `Answer` 发送到浏览器 —— 只发送 `GetPublicData()` 和图像。

完整 API、Redis 接入与评分细节见 [`v2/antibot/README.md`](v2/antibot/README.md)。

<br/>
<hr/>

## 验证码图像

### JPEGImageData

| Method                                                    | Desc                                                                                                                       |
|-----------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| Get() image.Image                                         | <span style='padding: 0 10px'></span>获取原图像                                                                                 |
| ToBytes() ([]byte, error)                                 | <span style='padding: 0 10px'></span>转为字节数组                                                                                |
| ToBytesWithQuality(imageQuality int) ([]byte, error)      | 指定清晰度转为字节数组                                                                                                                |
| ToBase64() (string, error)                                | <span style='padding: 0 10px'></span>转为 Base64 字符串，带 <span style='color:#ed4630;'>"data:image/jpeg;base64,"</span> 前缀      |
| ToBase64Data() (string, error)                            | <span style='padding: 0 10px'></span>转为 Base64 字符串                                                                         |
| ToBase64WithQuality(imageQuality int)  (string, error)    | <span style='padding: 0 10px'></span>指定清晰度转为 Base64 字符串，带 <span style='color:#ed4630;'>"data:image/jpeg;base64,"</span> 前缀 |
| ToBase64DataWithQuality(imageQuality int) (string, error) | <span style='padding: 0 10px'></span>指定清晰度转为 Base64 字符串                                                                    |
| SaveToFile(filepath string, quality int) error            | <span style='padding: 0 10px'></span>保存 JPEG 到文件                                                                           |


### PNGImageData

| Method                                    | Desc                                                                                                                 |
|-------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| Get() image.Image                         | <span style='padding: 0 10px'></span>获取原图像                                                                           |
| ToBytes() ([]byte, error)                 | <span style='padding: 0 10px'></span>转为字节数组                                                                          |
| ToBase64() (string, error)                | <span style='padding: 0 10px'></span>转为 Base64 字符串，带 <span style='color:#ed4630;'>"data:image/png;base64,"</span> 前缀 |
| ToBase64Data() (string, error)            | <span style='padding: 0 10px'></span>转为 Base64 字符串                                                                   |
| SaveToFile(filepath string) error         | <span style='padding: 0 10px'></span>保存 到文件                                                                          |


<br/>

## 验证模块
- [x] 文字点选
- [x] 图形点选
- [x] 滑动
- [x] 拖拽
- [x] 旋转
- [ ] 抛物线
- [ ] 看图选择
- [ ] 物品辨认
- [ ] ...

## 扩展&增强
- [x] 基本验证
- [ ] 行为检测增强
- [ ] 其他因素增强
- [ ] 多任务生成模式
- [ ] ...

## 语言支持
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
- [ ] WX-Applet
- [ ] React Native App
- [ ] Flutter App
- [ ] Android App
- [ ] IOS App
- ...

## Deployment Service
- [x] Binary Program
- [x] Docker Image
- ...

<br/>

## 赞助一下

<p>如果觉得项目有帮助，可以请作者喝杯咖啡 🍹</p>
<div>
<a href="http://witkeycode.com/sponsor" target="_blank"><img src="http://47.104.180.148/payment-code/wxpay.png" alt="Buy Me A Coffee" style="width: 217px !important;" ></a>
<a href="http://witkeycode.com/sponsor" target="_blank"><img src="http://47.104.180.148/payment-code/alipay.png" alt="Buy Me A Coffee" style="width: 217px !important;" ></a>
</div>

<br/>

## LICENSE
Go Captcha source code is licensed under the Apache Licence, Version 2.0 [http://www.apache.org/licenses/LICENSE-2.0.html](http://www.apache.org/licenses/LICENSE-2.0.html)

<br/>

