# eMMC153-UI

Инструмент разметки фото плат: JEDEC **eMMC153** (14×14), поиск ISP-пинов.

## GUI

```bash
go run ./cmd/ui
```

1. **Из буфера** (⌘V) — фото footprint  
2. **Сетка мышкой** — от центра **A1** до противоположного угла  
3. **Ключ A1** — клик по вырезу шёлка  
4. Подгони **Pitch** в полях  
5. **Найти пины** → сохранить PNG/JSON  

Сетка и маркеры — векторный оверлей поверх статичной картинки (без перерисовки растра).

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
