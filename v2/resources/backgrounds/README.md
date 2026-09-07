# Bundled backgrounds

Readable, landmark-rich backgrounds for GoCaptcha masters. Dense color and
texture across the frame help humans match the slide tile by image alignment;
all drop slots share the same silhouette, so visual content is the cue.

| File              | Theme                                         |
|-------------------|-----------------------------------------------|
| `bg_meadow.png`   | Sunny meadow, path, and a single tree         |
| `bg_lakeside.png` | Calm lake with pier and distant hills         |
| `bg_desert.png`   | Golden dunes with soft shadows                |
| `bg_flowers.png`  | Large blooms on a soft garden blur            |
| `bg_coast.png`    | Beach bands: sky, turquoise water, sand       |
| `bg_fruit.png`    | Dense fruit market colors                     |
| `bg_toys.png`     | Colorful wooden toys on a patterned quilt     |
| `bg_forest.png`   | Autumn forest path with leaf cover            |

All images are `720x540` RGB PNG. The library randomly crops a master-sized region
(default `300x220`) from each background, so larger source images add positional variety.

## Usage

```go
bg, _ := codec.DecodeByteToPng(mustRead("bg_fruit.png"))
builder.SetResources(slide.WithBackgrounds([]image.Image{bg}))
```
