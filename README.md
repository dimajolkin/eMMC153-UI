# eMMC153-UI

Разметка ISP-площадок JEDEC **eMMC153** (14×14) на фото footprint: GUI + CLI.

## GUI

```bash
go run ./cmd/ui
```

**Из буфера** (⌘V) или файл → ROI → **Разметить** → **Сохранить…**

## CLI

```bash
go run ./cmd/annotate -in photo.png -roi 240,240,400,400 -out out.png -bw mask.png
```

## ISP min set

| Signal | Ball |
|--------|------|
| CLK    | M6   |
| CMD    | M5   |
| DAT0   | A3   |
| VCC    | E6   |
| VCCQ   | M4   |
| GND    | A6   |

A1 = top-left (silkscreen notch).
