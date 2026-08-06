# eMMC153-UI

Разметка ISP-площадок JEDEC **eMMC153** (14×14) на фото footprint: GUI + CLI.

## GUI

```bash
go run ./cmd/ui
```

1. **Из буфера** (⌘V) — скрин footprint
2. **Сетка мышкой** — протяни от центра шара **A1** до противоположного угла (**P14**)
3. **Ключ A1** — кликни вырез на шёлке / первый пад
4. Подгони **A1 / Pitch X/Y** в полях
5. **Разобрать пины** → сохранить

## CLI

```bash
go run ./cmd/annotate -in photo.png -roi 240,240,400,400 -a1 260,271 -pitch 28.9 -out out.png
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
