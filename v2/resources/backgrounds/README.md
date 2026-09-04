# Bundled backgrounds

A fresh set of high-complexity, high-entropy background images for GoCaptcha masters.
They are intentionally busy (no large flat regions, no readable text) to raise the cost
of OCR / contour-based solvers.

| File              | Theme                                   |
|-------------------|-----------------------------------------|
| `bg_mosaic.png`   | Dense low-poly geometric mosaic         |
| `bg_foliage.png`  | Close-up tropical foliage               |
| `bg_circuit.png`  | Macro circuit board                     |
| `bg_graffiti.png` | Abstract graffiti splatter              |
| `bg_agate.png`    | Swirling agate / marble mineral veins   |
| `bg_autumn.png`   | Carpet of overlapping autumn leaves     |
| `bg_yarn.png`     | Tangled multicolor yarn strands         |
| `bg_city.png`     | High aerial view of a dense city        |

All images are `720x540` RGB PNG. The library randomly crops a master-sized region
(default `300x220`) from each background, so larger source images add positional variety.

## Usage

```go
bg, _ := codec.DecodeByteToPng(mustRead("bg_mosaic.png"))
builder.SetResources(click.WithBackgrounds([]image.Image{bg}))
```
