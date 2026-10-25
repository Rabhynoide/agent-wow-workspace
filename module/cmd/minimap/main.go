// minimap writes the client's minimap images and our road grids as map tiles for the dashboard:
// <out>/<map>/<z>/<x>_<y>.jpg (z 6: one 256-pixel tile per ADT, x = ADT x, y = ADT y; z 5..0: halved)
// and <out>/<map>/roads/<z>/<x>_<y>.png (transparent). go run ./cmd/minimap [-maps 0,1,530,571]
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"wowagent/internal/blp"
	"wowagent/internal/mpq"
	"wowagent/internal/roads"
)

var dirs = map[uint32]string{0: "Azeroth", 1: "Kalimdor", 530: "Expansion01", 571: "Northrend"}

const (
	tile = 256
	maxZ = 6 // 2^6 = 64 tiles per side
)

var background = color.NRGBA{16, 24, 32, 255}

func main() {
	client := flag.String("client", "../assets/client/Data", "client Data directory")
	roadsDir := flag.String("roads", "../assets/roads", "road grids (cmd/roads)")
	out := flag.String("out", "../assets/minimap", "output directory")
	maps := flag.String("maps", "0,1,530,571", "maps")
	flag.Parse()
	set, err := mpq.OpenClient(*client)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer set.Close()
	trs, err := set.Read(`textures\Minimap\md5translate.trs`)
	if err != nil {
		fmt.Println("md5translate:", err)
		os.Exit(1)
	}
	hashed := map[string]string{} // "azeroth\map32_48.blp" -> hashed file
	for _, l := range strings.Split(string(trs), "\n") {
		p := strings.Split(strings.TrimSpace(l), "\t")
		if len(p) == 2 {
			hashed[strings.ToLower(p[0])] = p[1]
		}
	}
	for _, s := range strings.Split(*maps, ",") {
		id64, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
		id := uint32(id64)
		dir := dirs[id]
		base := filepath.Join(*out, strconv.Itoa(int(id)))
		have := map[[2]int]bool{}
		for x := 0; x < 64; x++ {
			for y := 0; y < 64; y++ {
				h := hashed[strings.ToLower(fmt.Sprintf(`%s\map%02d_%02d.blp`, dir, x, y))]
				if h == "" {
					continue
				}
				b, err := set.Read(`textures\Minimap\` + h)
				if err != nil {
					continue
				}
				img, err := blp.Decode(b)
				if err != nil {
					slog.Warn("blp", "map", id, "x", x, "y", y, "err", err)
					continue
				}
				if err := save(filepath.Join(base, strconv.Itoa(maxZ), fmt.Sprintf("%d_%d.jpg", x, y)), flatten(scale(img, tile)), false); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				have[[2]int{x, y}] = true
			}
		}
		pyramid(base, have, false)
		rm, err := roads.Load(*roadsDir, id)
		if err != nil {
			slog.Warn("no roads", "map", id, "err", err)
		}
		rhave := map[[2]int]bool{}
		for k, g := range rm.Grids {
			// grid (gx, gy) is ADT (x, y) = (gy, gx); cell (lx, ly): lx down (south), ly right (east).
			img := image.NewNRGBA(image.Rect(0, 0, tile, tile))
			for lx := 0; lx < roads.Res; lx++ {
				for ly := 0; ly < roads.Res; ly++ {
					i := lx*roads.Res + ly
					if g[i/8]&(1<<(i%8)) == 0 {
						continue
					}
					for dy := 0; dy < 2; dy++ {
						for dx := 0; dx < 2; dx++ {
							img.SetNRGBA(ly*2+dx, lx*2+dy, color.NRGBA{255, 170, 40, 220})
						}
					}
				}
			}
			x, y := k[1], k[0]
			if err := save(filepath.Join(base, "roads", strconv.Itoa(maxZ), fmt.Sprintf("%d_%d.png", x, y)), img, true); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			rhave[[2]int{x, y}] = true
		}
		pyramid(filepath.Join(base, "roads"), rhave, true)
		slog.Info("minimap", "map", id, "tiles", len(have), "road tiles", len(rhave))
	}
}

// pyramid builds zoom levels maxZ-1..0 from the level below (four tiles into one).
func pyramid(base string, have map[[2]int]bool, alpha bool) {
	ext := ".jpg"
	if alpha {
		ext = ".png"
	}
	for z := maxZ - 1; z >= 0; z-- {
		next := map[[2]int]bool{}
		for k := range have {
			next[[2]int{k[0] / 2, k[1] / 2}] = true
		}
		for k := range next {
			img := image.NewNRGBA(image.Rect(0, 0, 2*tile, 2*tile))
			if !alpha {
				fill(img, background)
			}
			for dx := 0; dx < 2; dx++ {
				for dy := 0; dy < 2; dy++ {
					c := [2]int{2*k[0] + dx, 2*k[1] + dy}
					if !have[c] {
						continue
					}
					sub, err := load(filepath.Join(base, strconv.Itoa(z+1), fmt.Sprintf("%d_%d%s", c[0], c[1], ext)))
					if err != nil {
						continue
					}
					for y := 0; y < tile; y++ {
						for x := 0; x < tile; x++ {
							img.Set(dx*tile+x, dy*tile+y, sub.At(sub.Bounds().Min.X+x, sub.Bounds().Min.Y+y))
						}
					}
				}
			}
			save(filepath.Join(base, strconv.Itoa(z), fmt.Sprintf("%d_%d%s", k[0], k[1], ext)), scale(img, tile), alpha)
		}
		have = next
	}
}

func fill(img *image.NRGBA, c color.NRGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
}

// flatten puts the image on the background (minimap tiles have transparent water).
func flatten(img *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(img.Bounds())
	for i := 0; i < len(img.Pix); i += 4 {
		a := int(img.Pix[i+3])
		for c, bg := range []int{int(background.R), int(background.G), int(background.B)} {
			out.Pix[i+c] = byte((int(img.Pix[i+c])*a + bg*(255-a)) / 255)
		}
		out.Pix[i+3] = 255
	}
	return out
}

// scale resizes a square image to n × n (box filter).
func scale(img *image.NRGBA, n int) *image.NRGBA {
	w := img.Bounds().Dx()
	if w == n {
		return img
	}
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	f := w / n
	if f < 1 {
		f = 1
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a, cnt int
			for dy := 0; dy < f; dy++ {
				for dx := 0; dx < f; dx++ {
					o := img.PixOffset(x*w/n+dx, y*w/n+dy)
					pa := int(img.Pix[o+3])
					r += int(img.Pix[o]) * pa
					g += int(img.Pix[o+1]) * pa
					b += int(img.Pix[o+2]) * pa
					a += pa
					cnt++
				}
			}
			o := out.PixOffset(x, y)
			if a > 0 {
				out.Pix[o], out.Pix[o+1], out.Pix[o+2] = byte(r/a), byte(g/a), byte(b/a)
			}
			// Roads stay visible when zoomed out: keep the strongest coverage, not the mean.
			out.Pix[o+3] = byte(min(255, 2*a/cnt))
		}
	}
	return out
}

func save(path string, img image.Image, alpha bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if alpha {
		return png.Encode(f, img)
	}
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 82})
}

func load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
