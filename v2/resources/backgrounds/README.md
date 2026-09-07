# Bundled backgrounds

Readable, landmark-rich backgrounds for GoCaptcha masters. Dense color and
texture across the frame help humans match the slide tile by image alignment;
all drop slots share the same silhouette, so visual content is the cue.

| File                   | Theme                                              |
|------------------------|----------------------------------------------------|
| `bg_reef.png`          | Underwater reef with turtles, rays, and ruins      |
| `bg_underwater.png`    | Coral canyon leading to a sunken temple            |
| `bg_neon_city.png`     | Neon cyberpunk canyon with screens and drone       |
| `bg_crystal_city.png`  | Futuristic city with floating crystals             |
| `bg_rooftops.png`      | Sunny East Asian rooftops, lanterns, and market    |
| `bg_riverside.png`     | Riverside town with boats and mountain backdrop    |
| `bg_matsuri.png`       | Night festival stalls, lanterns, and goldfish tubs |
| `bg_winter_fair.png`   | Snowy Russian fair with carousel and onion domes   |
| `bg_picnic.png`        | Garden picnic with animals, quilt, and treehouse   |

All images are square RGB PNG (1024×1024 or 1536×1536). The library randomly
crops a master-sized region (default `300x220`) and prefers high-variance /
textured areas so slide notches stay readable on dense illustration art.

## Usage

```go
bg, _ := codec.DecodeByteToPng(mustRead("bg_reef.png"))
builder.SetResources(slide.WithBackgrounds([]image.Image{bg}))
```
