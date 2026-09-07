# Bundled backgrounds

Readable, landmark-rich backgrounds for GoCaptcha masters. Large color regions
and clear edges help humans match the slide tile by image alignment; all drop
slots share the same silhouette, so visual content is the cue.

| File              | Theme                                         | Best for   |
|-------------------|-----------------------------------------------|------------|
| `bg_meadow.png`   | Sunny meadow, path, and a single tree         | slide/rot  |
| `bg_lakeside.png` | Calm lake with pier and distant hills         | rotate     |
| `bg_desert.png`   | Golden dunes with soft shadows                | rotate     |
| `bg_flowers.png`  | Large blooms on a soft garden blur            | slide/rot  |
| `bg_coast.png`    | Beach bands: sky, turquoise water, sand       | rotate     |
| `bg_fruit.png`    | Dense fruit market colors                     | slide      |
| `bg_toys.png`     | Colorful wooden toys on a patterned quilt     | slide      |
| `bg_forest.png`   | Autumn forest path with leaf cover            | slide      |

Slide generation also bias-crops toward high-texture regions so notches avoid
empty sky/water. The demo wires dense assets for slide/drag and keeps scenic
horizons available for rotate.

All images are `720x540` RGB PNG. The library randomly crops a master-sized region
(default `300x220`) from each background, so larger source images add positional variety.

## Usage

```go
bg, _ := codec.DecodeByteToPng(mustRead("bg_fruit.png"))
builder.SetResources(slide.WithBackgrounds([]image.Image{bg}))
```
