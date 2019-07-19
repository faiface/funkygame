package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"math"
	"os"

	"github.com/faiface/funky"
	"github.com/faiface/funky/runtime"
	"github.com/golang/freetype/truetype"
	"github.com/hajimehoshi/ebiten"
	"github.com/hajimehoshi/ebiten/ebitenutil"
	"github.com/hajimehoshi/ebiten/inpututil"
	"github.com/hajimehoshi/ebiten/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

type config struct {
	title         string
	width, height int
	scale         float64
	images        []*ebiten.Image
	fonts         []font.Face
}

func main() {
	program, cleanup := funky.Run("main")
	defer cleanup()
	runLoop(runLoader(program))
}

func check(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	os.Exit(1)
}

func log(kind, msg string) {
	if kind == "" {
		fmt.Printf("%s\n", msg)
	} else {
		fmt.Printf("%s: %s\n", kind, msg)
	}
}

func runLoader(program *runtime.Value) (*config, *runtime.Value) {
	var cfg config
	cfg.fonts = append(cfg.fonts, basicfont.Face7x13)

	for {
		switch program.Alternative() {
		// quit
		case 0:
			log("", "QUIT")
			os.Exit(0)

		// log
		case 1:
			log("LOG", program.Field(0).String())
			program = program.Field(1)

		// generate-image
		case 2:
			var (
				width    = toInt(program.Field(0))
				height   = toInt(program.Field(1))
				function = program.Field(2)
			)
			log("GENERATE", fmt.Sprintf("%dx%d", width, height))
			source := image.NewRGBA(image.Rect(0, 0, width, height))
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					pt := mkPoint(float64(x), float64(y))
					clr := toRGBA(function.Apply(pt))
					source.SetRGBA(x, y, clr)
				}
			}
			img, err := ebiten.NewImageFromImage(source, ebiten.FilterDefault)
			check(err)
			program = program.Field(3).Apply(mkImage(&cfg, img))

		// load-image
		case 3:
			path := program.Field(0).String()
			log("LOAD", path)
			img, _, err := ebitenutil.NewImageFromFile(path, ebiten.FilterDefault)
			check(err)
			program = program.Field(1).Apply(mkImage(&cfg, img))

		// load-sheet
		case 4:
			var (
				path  = program.Field(0).String()
				tileW = toInt(program.Field(1))
				tileH = toInt(program.Field(2))
			)
			log("LOAD", path)
			sheet, _, err := ebitenutil.NewImageFromFile(path, ebiten.FilterDefault)
			check(err)
			var rows []*runtime.Value
			for y := 0; y < sheet.Bounds().Dy(); y += tileH {
				var row []*runtime.Value
				for x := 0; y < sheet.Bounds().Dx(); x += tileW {
					tile := sheet.SubImage(image.Rect(x, y, x+tileW, y+tileH)).(*ebiten.Image)
					row = append(row, mkImage(&cfg, tile))
				}
				rows = append(rows, runtime.MkList(row...))
			}
			program = program.Field(3).Apply(runtime.MkList(rows...))

		// load-font
		case 5:
			var (
				path = program.Field(0).String()
				size = program.Field(1).Float()
			)
			log("LOAD", path)
			face, err := loadFace(path, size)
			check(err)
			program = program.Field(3).Apply(mkFont(&cfg, face, size))

		// load-text
		case 6:
			path := program.Field(0).String()
			log("LOAD", path)
			content, err := ioutil.ReadFile(path)
			check(err)
			program = program.Field(1).Apply(runtime.MkString(string(content)))

		// start
		case 7:
			cfg.title = program.Field(0).String()
			cfg.width = toInt(program.Field(1))
			cfg.height = toInt(program.Field(2))
			cfg.scale = program.Field(3).Float()
			return &cfg, program.Field(4)

		default:
			panic("impossible")
		}
	}
}

func runLoop(cfg *config, program *runtime.Value) {
	update := func(screen *ebiten.Image) error {
		for {
			switch program.Alternative() {
			// quit
			case 0:
				log("", "QUIT")
				return io.EOF

			// log
			case 1:
				log("LOG", program.Field(0).String())
				program = program.Field(1)

			// fill
			case 2:
				if !ebiten.IsDrawingSkipped() {
					clr := toRGBA(program.Field(0))
					screen.Fill(clr)
				}
				program = program.Field(1)

			// draw-sprite
			case 3:
				if !ebiten.IsDrawingSkipped() {
					sprite := program.Field(0)
					var (
						imageID          = toInt(sprite.Field(0).Field(0))
						img              = cfg.images[imageID]
						filter           = toFilter(sprite.Field(1))
						mask             = toRGBA(sprite.Field(2))
						posX, posY       = toPoint(sprite.Field(3))
						align            = sprite.Field(4).Alternative()
						anchorX, anchorY = alignmentToAnchor(align, img.Bounds().Dx(), img.Bounds().Dy())
						rotation         = sprite.Field(5).Float()
						scale            = sprite.Field(6).Float()
					)
					var opts ebiten.DrawImageOptions
					opts.Filter = filter
					opts.GeoM.Translate(-anchorX, -anchorY)
					opts.GeoM.Rotate(rotation)
					opts.GeoM.Scale(scale, scale)
					opts.GeoM.Translate(posX, posY)
					opts.ColorM.Scale(
						float64(mask.R)/255,
						float64(mask.G)/255,
						float64(mask.B)/255,
						float64(mask.A)/255,
					)
					screen.DrawImage(img, &opts)
				}
				program = program.Field(1)

			// draw-text
			case 4:
				if !ebiten.IsDrawingSkipped() {
					var (
						s                = program.Field(0).String()
						fontID           = toInt(program.Field(1).Field(0))
						face             = cfg.fonts[fontID]
						clr              = toRGBA(program.Field(2))
						posX, posY       = toPoint(program.Field(3))
						align            = program.Field(4).Alternative()
						width            = font.MeasureString(face, s).Round()
						height           = face.Metrics().Height.Round()
						anchorX, anchorY = alignmentToAnchor(align, width, height)
					)
					text.Draw(screen, s, face, int(math.Round(posX-anchorX)), int(math.Round(posY-anchorY))+height, clr)
				}
				program = program.Field(5)

			// present
			case 5:
				log("FRAME", fmt.Sprintf("%.1f FPS", ebiten.CurrentFPS()))

				mouseX, mouseY := ebiten.CursorPosition()
				program = program.Field(0).Apply(runtime.MkRecord(
					keyState(ebiten.KeyLeft),
					keyState(ebiten.KeyRight),
					keyState(ebiten.KeyUp),
					keyState(ebiten.KeyDown),

					keyState(ebiten.KeyEnter),
					keyState(ebiten.KeyEscape),
					keyState(ebiten.KeySpace),
					keyState(ebiten.KeyBackspace),

					keyState(ebiten.KeyW),
					keyState(ebiten.KeyA),
					keyState(ebiten.KeyS),
					keyState(ebiten.KeyD),

					keyState(ebiten.KeyI),
					keyState(ebiten.KeyJ),
					keyState(ebiten.KeyK),
					keyState(ebiten.KeyL),

					mkPoint(float64(mouseX), float64(mouseY)),
					mouseButtonState(ebiten.MouseButtonLeft),
					mouseButtonState(ebiten.MouseButtonRight),
				))

				return nil

			default:
				panic("impossible")
			}
		}
	}

	check(ebiten.Run(update, cfg.width, cfg.height, cfg.scale, cfg.title))
}

func mkInt(x int) *runtime.Value {
	return runtime.MkInt64(int64(x))
}

func toInt(val *runtime.Value) int {
	return int(val.Int().Int64())
}

func toFilter(val *runtime.Value) ebiten.Filter {
	switch val.Alternative() {
	case 0: // nearest
		return ebiten.FilterNearest
	case 1: // linear
		return ebiten.FilterLinear
	default:
		panic("impossible")
	}
}

func mkPoint(x, y float64) *runtime.Value {
	return runtime.MkRecord(
		runtime.MkFloat(x),
		runtime.MkFloat(y),
	)
}

func toPoint(val *runtime.Value) (x, y float64) {
	x = val.Field(0).Float()
	y = val.Field(1).Float()
	return
}

func toRGBA(val *runtime.Value) color.RGBA {
	r := uint8(val.Field(0).Float() * 255)
	g := uint8(val.Field(1).Float() * 255)
	b := uint8(val.Field(2).Float() * 255)
	a := uint8(val.Field(3).Float() * 255)
	return color.RGBA{r, g, b, a}
}

func mkImage(cfg *config, img *ebiten.Image) *runtime.Value {
	var (
		id = len(cfg.images)
		w  = img.Bounds().Dx()
		h  = img.Bounds().Dy()
	)
	cfg.images = append(cfg.images, img)
	return runtime.MkRecord(
		mkInt(id),
		mkInt(w),
		mkInt(h),
	)
}

func mkFont(cfg *config, face font.Face, size float64) *runtime.Value {
	id := len(cfg.fonts)
	cfg.fonts = append(cfg.fonts, face)
	return runtime.MkRecord(
		mkInt(id),
		runtime.MkFloat(size),
	)
}

func loadFace(path string, size float64) (font.Face, error) {
	ttf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	font, err := truetype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	return truetype.NewFace(font, &truetype.Options{Size: size}), nil
}

const (
	alignTopLeft = iota
	alignTop
	alignTopRight
	alignLeft
	alignCenter
	alignRight
	alignBottomLeft
	alignBottom
	alignBottomRight
)

func alignmentToAnchor(align int, w, h int) (x, y float64) {
	switch align {
	case alignTopLeft, alignLeft, alignBottomLeft:
		x = 0
	case alignTop, alignCenter, alignBottom:
		x = float64(w) / 2
	case alignTopRight, alignRight, alignBottomRight:
		x = float64(w)
	}
	switch align {
	case alignTopLeft, alignTop, alignTopRight:
		y = 0
	case alignLeft, alignCenter, alignRight:
		y = float64(h) / 2
	case alignBottomLeft, alignBottom, alignBottomRight:
		y = float64(h)
	}
	return
}

func keyState(key ebiten.Key) *runtime.Value {
	if inpututil.IsKeyJustPressed(key) {
		return runtime.MkUnion(0)
	}
	if ebiten.IsKeyPressed(key) {
		return runtime.MkUnion(1)
	}
	if inpututil.IsKeyJustReleased(key) {
		return runtime.MkUnion(2)
	}
	return runtime.MkUnion(3)
}

func mouseButtonState(btn ebiten.MouseButton) *runtime.Value {
	if inpututil.IsMouseButtonJustPressed(btn) {
		return runtime.MkUnion(0)
	}
	if ebiten.IsMouseButtonPressed(btn) {
		return runtime.MkUnion(1)
	}
	if inpututil.IsMouseButtonJustReleased(btn) {
		return runtime.MkUnion(2)
	}
	return runtime.MkUnion(3)
}
