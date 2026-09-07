# AntiBot UI demo

Local page with **Rotate**, **Slide**, and **Drag-Drop**. Slide includes a **horizontal slider** so the tile can be moved left–right (same idea as Rotate’s angle track).

```bash
cd v2/demo
go run .
# open http://127.0.0.1:8080
```

Optional: `PORT=9000 go run .`

Backgrounds are loaded from `../resources/backgrounds/bg_*.png`. Slide/drag use three synthesized tile graphs; only one slot matches the piece.
