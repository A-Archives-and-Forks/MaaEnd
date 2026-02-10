package mapservice

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"
)

// ==========================================
// 基础图像工具
// ==========================================

// EnsureRGBA 将任意图像转换为 *image.RGBA
func EnsureRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			dst.Set(x, y, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}

// copySubImage 从源图像的指定矩形区域创建一个新的 RGBA 图像。
func copySubImage(src *image.RGBA, r image.Rectangle) *image.RGBA {
	w, h := r.Dx(), r.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	srcStride := src.Stride
	dstStride := dst.Stride
	srcPix := src.Pix
	dstPix := dst.Pix

	srcBase := src.PixOffset(r.Min.X, r.Min.Y)

	for y := 0; y < h; y++ {
		copy(dstPix[y*dstStride:y*dstStride+w*4], srcPix[srcBase+y*srcStride:srcBase+y*srcStride+w*4])
	}

	return dst
}

// DownscaleRGBA 分配一个新图像并使用最近邻插值进行降采样。
func DownscaleRGBA(img image.Image, scale int) *image.RGBA {
	bounds := img.Bounds()
	newW := bounds.Dx() / scale
	newH := bounds.Dy() / scale
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	DownscaleRGBAInto(img, dst, scale)
	return dst
}

// DownscaleRGBAInto 使用最近邻插值将 src 降采样到 dst。
// dst 的边界必须匹配 src 边界 / 缩放比例。
func DownscaleRGBAInto(img image.Image, dst *image.RGBA, scale int) {
	bounds := img.Bounds()
	newW := dst.Bounds().Dx()
	newH := dst.Bounds().Dy()

	// RGBA 快速路径
	if src, ok := img.(*image.RGBA); ok {
		srcStride := src.Stride
		dstStride := dst.Stride
		srcPix := src.Pix
		dstPix := dst.Pix

		for y := 0; y < newH; y++ {
			// Src Y coordinate
			srcY := bounds.Min.Y + y*scale
			srcRowY := srcY - src.Rect.Min.Y
			if srcRowY < 0 {
				continue
			}
			srcRowStart := srcRowY * srcStride
			dstRowOffset := y * dstStride

			for x := 0; x < newW; x++ {
				srcX := bounds.Min.X + x*scale
				srcColX := srcX - src.Rect.Min.X
				srcOffset := srcRowStart + srcColX*4
				dstOffset := dstRowOffset + x*4
				copy(dstPix[dstOffset:dstOffset+4], srcPix[srcOffset:srcOffset+4])
			}
		}
		return
	}

	// 其他慢速路径
	for y := 0; y < newH; y++ {
		srcY := bounds.Min.Y + y*scale
		for x := 0; x < newW; x++ {
			srcX := bounds.Min.X + x*scale
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}
}

// ==========================================
// 几何与绘图
// ==========================================

func checkBounds(x, y, w, h int) bool {
	return x >= 0 && x < w && y >= 0 && y < h
}

func drawRect(dst *image.RGBA, x1, y1, x2, y2 int, c color.Color) {
	bounds := dst.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if x1 < 0 {
		x1 = 0
	}
	if y1 < 0 {
		y1 = 0
	}
	if x2 > w {
		x2 = w
	}
	if y2 > h {
		y2 = h
	}

	// Top & Bottom
	for x := x1; x < x2; x++ {
		if checkBounds(x, y1, w, h) {
			dst.Set(x, y1, c)
		}
		if checkBounds(x, y2-1, w, h) {
			dst.Set(x, y2-1, c)
		}
	}
	// Left & Right
	for y := y1; y < y2; y++ {
		if checkBounds(x1, y, w, h) {
			dst.Set(x1, y, c)
		}
		if checkBounds(x2-1, y, w, h) {
			dst.Set(x2-1, y, c)
		}
	}
}

// GenerateCircularMask 为圆形小地图创建 Alpha 蒙版
func GenerateCircularMask(w, h int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, w, h))

	// 外半径（保留地图区域）
	radius := float64(w) / 2
	if float64(h)/2 < radius {
		radius = float64(h) / 2
	}

	// 内半径（移除玩家箭头）
	innerRadius := 10.0

	cx, cy := float64(w)/2, float64(h)/2

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x) - cx + 0.5
			dy := float64(y) - cy + 0.5
			distSq := dx*dx + dy*dy

			// 在外半径内 且 在内半径外
			if distSq <= radius*radius && distSq > innerRadius*innerRadius {
				mask.SetAlpha(x, y, color.Alpha{255}) // 有效
			} else {
				mask.SetAlpha(x, y, color.Alpha{0}) // 忽略（中心 + 边缘）
			}
		}
	}
	return mask
}

// ApplyMaskFastInto 将 Alpha 蒙版应用到图像并绘制到 dst。
func ApplyMaskFastInto(src image.Image, dst *image.RGBA, mask *image.Alpha) {
	bounds := src.Bounds()
	// 将 src 绘制到 dst
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	// 应用 alpha 蒙版
	draw.DrawMask(dst, dst.Bounds(), src, bounds.Min, mask, image.Point{}, draw.Src)
}

// ==========================================
// 图像过滤/遮罩
// ==========================================

// ApplySpotlightEffect 通过透明化移除 Tier 地图中的暗区（空白）。
func ApplySpotlightEffect(img *image.RGBA, threshold int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	pix := img.Pix
	stride := img.Stride

	for y := 0; y < h; y++ {
		rowOffset := y * stride
		for x := 0; x < w; x++ {
			offset := rowOffset + x*4

			// 如果已经是透明的，则跳过
			if pix[offset+3] == 0 {
				continue
			}

			r := int(pix[offset+0])
			g := int(pix[offset+1])
			b := int(pix[offset+2])

			luma := (r*3 + g*6 + b) / 10

			if luma < threshold {
				// 设置 Alpha 为 0（透明）
				pix[offset+3] = 0
				// 可选：清除颜色通道以便调试/保持整洁
				pix[offset+0] = 0
				pix[offset+1] = 0
				pix[offset+2] = 0
			}
		}
	}
}

// ApplyVoidFilter 扫描图像并将暗区（低于阈值）替换为透明 Alpha (0)。
func ApplyVoidFilter(img *image.RGBA, threshold int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	pix := img.Pix
	stride := img.Stride

	for y := 0; y < h; y++ {
		rowOffset := y * stride
		for x := 0; x < w; x++ {
			offset := rowOffset + x*4

			r := int(pix[offset+0])
			g := int(pix[offset+1])
			b := int(pix[offset+2])

			luma := (r*3 + g*6 + b) / 10

			if luma < threshold {
				// 设置 Alpha 为 0（透明）
				pix[offset+3] = 0
				// 清除 RGB 通道
				pix[offset+0] = 0
				pix[offset+1] = 0
				pix[offset+2] = 0
			}
		}
	}
}

// ==========================================
// 模板匹配（核心算法）
// ==========================================

type ProbePoint struct {
	X, Y    int
	R, G, B int
	Sat     int // 饱和度 = max(R,G,B) - min(R,G,B)
	GradMag int // 梯度幅度 = |gradX| + |gradY|
}

type TemplateProbe struct {
	Points []ProbePoint
	Width  int
	Height int
}

func NewTemplateProbe() *TemplateProbe {
	return &TemplateProbe{
		Points: make([]ProbePoint, 0, 4096),
	}
}

// UpdateFromMinimap
// 1. mask
// 2. maskIcons
// 3. 计算饱和度和梯度
// 4. 存入Probe
func (tp *TemplateProbe) UpdateFromMinimap(img *image.RGBA, mask *image.Alpha) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	tp.Width = w
	tp.Height = h

	tp.Points = tp.Points[:0]

	pix := img.Pix
	stride := img.Stride

	maskPix := mask.Pix
	maskStride := mask.Stride

	const DiffThreshold = 40

	abs := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}

	// 留1px边界用于梯度计算
	for y := 1; y < h-1; y++ {
		rowOffset := y * stride
		maskOffset := y * maskStride

		for x := 1; x < w-1; x++ {
			if maskPix[maskOffset+x] == 0 {
				continue
			}

			offset := rowOffset + x*4
			if pix[offset+3] == 0 {
				continue
			}

			r := int(pix[offset+0])
			g := int(pix[offset+1])
			b := int(pix[offset+2])

			isIcon := false

			// Icon过滤
			if r > 100 && g > 100 {
				minRG := r
				if g < minRG {
					minRG = g
				}
				if (minRG - b) > DiffThreshold {
					isIcon = true
				}
			}
			if !isIcon && b > 100 {
				maxRG := r
				if g > maxRG {
					maxRG = g
				}
				if (b - maxRG) > DiffThreshold {
					isIcon = true
				}
			}

			if isIcon {
				continue
			}

			// 计算饱和度
			maxC, minC := r, r
			if g > maxC {
				maxC = g
			}
			if b > maxC {
				maxC = b
			}
			if g < minC {
				minC = g
			}
			if b < minC {
				minC = b
			}
			sat := maxC - minC

			// 计算梯度幅度
			offsetLeft := rowOffset + (x-1)*4
			offsetRight := rowOffset + (x+1)*4
			grayLeft := (int(pix[offsetLeft])*3 + int(pix[offsetLeft+1])*6 + int(pix[offsetLeft+2])) / 10
			grayRight := (int(pix[offsetRight])*3 + int(pix[offsetRight+1])*6 + int(pix[offsetRight+2])) / 10
			gradX := grayRight - grayLeft

			offsetUp := (y-1)*stride + x*4
			offsetDown := (y+1)*stride + x*4
			grayUp := (int(pix[offsetUp])*3 + int(pix[offsetUp+1])*6 + int(pix[offsetUp+2])) / 10
			grayDown := (int(pix[offsetDown])*3 + int(pix[offsetDown+1])*6 + int(pix[offsetDown+2])) / 10
			gradY := grayDown - grayUp

			gradMag := abs(gradX) + abs(gradY)

			tp.Points = append(tp.Points, ProbePoint{
				X: x, Y: y,
				R: r, G: g, B: b,
				Sat:     sat,
				GradMag: gradMag,
			})
		}
	}
}

// MatchProbe 匹配，返回最佳匹配位置和平均差异
// step: 物理步进， probeStep: 采样步进
func MatchProbe(img *image.RGBA, probe *TemplateProbe, step int, probeStep int, useConcurrency bool) (bestX, bestY int, avgDiff float64) {
	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()

	maxX := imgW - probe.Width
	maxY := imgH - probe.Height
	if maxX <= 0 || maxY <= 0 {
		return 0, 0, 0
	}

	imgPix := img.Pix
	imgStride := img.Stride
	points := probe.Points

	validPixels := len(points) / probeStep
	if validPixels == 0 {
		return 0, 0, 0
	}

	abs := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}

	// 色调容忍度
	const ChromaThreshold = 45

	// 惩罚权重
	const ChromaWeight = 15

	matchRect := func(startX, endX, startY, endY int) (int, int, int) {
		localMinSAD := math.MaxInt64
		localX, localY := 0, 0

		for y := startY; y < endY; y += step {
			rowBase := y * imgStride
			for x := startX; x < endX; x += step {
				baseOffset := rowBase + x*4
				currentSAD := 0
				validCount := 0

				for i := 0; i < len(points); i += probeStep {
					p := &points[i]

					off := baseOffset + (p.Y * imgStride) + (p.X * 4)

					if off < 0 || off+2 >= len(imgPix) {
						continue
					}

					validCount++

					r := int(imgPix[off])
					g := int(imgPix[off+1])
					b := int(imgPix[off+2])

					diffR := abs(r - p.R)
					diffG := abs(g - p.G)
					diffB := abs(b - p.B)
					baseDiff := diffR + diffG + diffB

					pRG := p.R - p.G
					pBG := p.B - p.G
					mRG := r - g
					mBG := b - g

					chromaDiff := abs(pRG-mRG) + abs(pBG-mBG)

					// 非线性惩罚
					penalty := 0
					if chromaDiff > ChromaThreshold {
						penalty = (chromaDiff - ChromaThreshold) * ChromaWeight
					}

					// 饱和度不匹配惩罚：彩色小地图 vs 灰色地图
					satPenalty := 0
					if p.Sat > 30 { // 降低阈值
						// 计算地图像素饱和度
						mapMax, mapMin := r, r
						if g > mapMax {
							mapMax = g
						}
						if b > mapMax {
							mapMax = b
						}
						if g < mapMin {
							mapMin = g
						}
						if b < mapMin {
							mapMin = b
						}
						mapSat := mapMax - mapMin

						// 地图像素灰色时惩罚
						if mapSat < 25 {
							satPenalty = (p.Sat - mapSat) * 6 // 增加权重
						}
					}

					// 梯度惩罚：小地图有边缘 + 地图无边缘
					gradPenalty := 0
					if p.GradMag > 15 { // 降低阈值
						mapX := x + p.X
						mapY := y + p.Y
						if mapX > 0 && mapX < imgW-1 && mapY > 0 && mapY < imgH-1 {
							// 计算地图梯度幅度
							offLeft := baseOffset + (p.Y * imgStride) + ((p.X - 1) * 4)
							offRight := baseOffset + (p.Y * imgStride) + ((p.X + 1) * 4)
							if offLeft >= 0 && offRight+2 < len(imgPix) {
								grayLeft := (int(imgPix[offLeft])*3 + int(imgPix[offLeft+1])*6 + int(imgPix[offLeft+2])) / 10
								grayRight := (int(imgPix[offRight])*3 + int(imgPix[offRight+1])*6 + int(imgPix[offRight+2])) / 10
								mapGradX := abs(grayRight - grayLeft)

								offUp := baseOffset + ((p.Y - 1) * imgStride) + (p.X * 4)
								offDown := baseOffset + ((p.Y + 1) * imgStride) + (p.X * 4)
								if offUp >= 0 && offDown+2 < len(imgPix) {
									grayUp := (int(imgPix[offUp])*3 + int(imgPix[offUp+1])*6 + int(imgPix[offUp+2])) / 10
									grayDown := (int(imgPix[offDown])*3 + int(imgPix[offDown+1])*6 + int(imgPix[offDown+2])) / 10
									mapGradY := abs(grayDown - grayUp)

									mapGradMag := mapGradX + mapGradY

									// 地图无边缘时惩罚
									if mapGradMag < 15 {
										gradPenalty = p.GradMag - mapGradMag // 移除/2
									}
								}
							}
						}
					}

					currentSAD += baseDiff + penalty + satPenalty + gradPenalty

					if currentSAD > localMinSAD {
						break
					}
				}

				// 边缘检查
				minRequired := (len(points) * 85) / (probeStep * 100)
				if validCount < minRequired {
					currentSAD = math.MaxInt32
					continue
				}

				if currentSAD < localMinSAD {
					localMinSAD = currentSAD
					localX = x
					localY = y
				}
			}
		}
		return localX, localY, localMinSAD
	}

	if !useConcurrency {
		bx, by, sad := matchRect(0, maxX, 0, maxY)
		return bx, by, calcAvgDiff(sad, validPixels)
	}

	var mutex sync.Mutex
	globalMinSAD := math.MaxInt64
	globalX, globalY := 0, 0
	numWorkers := 8
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	rowsPerWorker := (maxY/step + 1 + numWorkers - 1) / numWorkers

	for i := 0; i < numWorkers; i++ {
		startIdx := i * rowsPerWorker
		endIdx := (i + 1) * rowsPerWorker
		go func(sIdx, eIdx int) {
			defer wg.Done()
			yStart := sIdx * step
			yEnd := eIdx * step
			if yEnd > maxY {
				yEnd = maxY + 1
			}
			lx, ly, lSad := matchRect(0, maxX, yStart, yEnd)
			mutex.Lock()
			if lSad < globalMinSAD {
				globalMinSAD = lSad
				globalX = lx
				globalY = ly
			}
			mutex.Unlock()
		}(startIdx, endIdx)
	}
	wg.Wait()

	return globalX, globalY, calcAvgDiff(globalMinSAD, validPixels)
}

// calcAvgDiff 计算平均差异（越小越好）
func calcAvgDiff(sad int, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(sad) / float64(count*3)
}

// ==========================================
// 边缘加权匹配算法（三重验证系统 - 策略1）
// ==========================================

// MatchProbeWeighted 边缘加权匹配，返回最佳匹配位置和加权平均差异
// 核心思想：边缘/纹理区域比平坦区域更有区分度
// gamma: 权重增长指数 (1=线性, 2=二次方推荐, 3=激进)
func MatchProbeWeighted(img *image.RGBA, probe *TemplateProbe, step int, probeStep int, useConcurrency bool, gamma float64) (bestX, bestY int, weightedAvgDiff float64) {
	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()

	maxX := imgW - probe.Width
	maxY := imgH - probe.Height
	if maxX <= 0 || maxY <= 0 {
		return 0, 0, 0
	}

	imgPix := img.Pix
	imgStride := img.Stride
	points := probe.Points

	// 预计算权重总和和加权点数
	var totalWeight float64
	validPixels := 0
	for i := 0; i < len(points); i += probeStep {
		p := &points[i]
		// 边缘权重：(gradMag / 255)^gamma，最小值0.1防止平坦区域被完全忽略
		w := math.Pow(float64(p.GradMag)/255.0, gamma)
		if w < 0.1 {
			w = 0.1 // 最小权重，保证平坦区域仍有贡献
		}
		totalWeight += w
		validPixels++
	}

	if validPixels == 0 || totalWeight < 0.001 {
		return 0, 0, 0
	}

	abs := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}

	// 色调容忍度
	const ChromaThreshold = 45
	const ChromaWeight = 15

	matchRect := func(startX, endX, startY, endY int) (int, int, float64) {
		localMinWeightedSAD := math.MaxFloat64
		localX, localY := 0, 0

		for y := startY; y < endY; y += step {
			rowBase := y * imgStride
			for x := startX; x < endX; x += step {
				baseOffset := rowBase + x*4
				weightedSAD := 0.0
				localTotalWeight := 0.0
				validCount := 0

				for i := 0; i < len(points); i += probeStep {
					p := &points[i]

					off := baseOffset + (p.Y * imgStride) + (p.X * 4)

					if off < 0 || off+2 >= len(imgPix) {
						continue
					}

					validCount++

					r := int(imgPix[off])
					g := int(imgPix[off+1])
					b := int(imgPix[off+2])

					diffR := abs(r - p.R)
					diffG := abs(g - p.G)
					diffB := abs(b - p.B)
					baseDiff := float64(diffR + diffG + diffB)

					// 色调惩罚
					pRG := p.R - p.G
					pBG := p.B - p.G
					mRG := r - g
					mBG := b - g
					chromaDiff := abs(pRG-mRG) + abs(pBG-mBG)

					penalty := 0.0
					if chromaDiff > ChromaThreshold {
						penalty = float64((chromaDiff - ChromaThreshold) * ChromaWeight)
					}

					// 饱和度惩罚
					satPenalty := 0.0
					if p.Sat > 30 {
						mapMax, mapMin := r, r
						if g > mapMax {
							mapMax = g
						}
						if b > mapMax {
							mapMax = b
						}
						if g < mapMin {
							mapMin = g
						}
						if b < mapMin {
							mapMin = b
						}
						mapSat := mapMax - mapMin
						if mapSat < 25 {
							satPenalty = float64((p.Sat - mapSat) * 6)
						}
					}

					totalDiff := baseDiff + penalty + satPenalty

					// === 核心改进：边缘加权 ===
					// 边缘权重：(gradMag / 255)^gamma
					weight := math.Pow(float64(p.GradMag)/255.0, gamma)
					if weight < 0.1 {
						weight = 0.1
					}

					weightedSAD += totalDiff * weight
					localTotalWeight += weight
				}

				// 边缘检查
				minRequired := (len(points) * 85) / (probeStep * 100)
				if validCount < minRequired {
					continue
				}

				// 归一化：加权平均差异
				if localTotalWeight > 0.001 {
					normalizedSAD := weightedSAD / localTotalWeight
					if normalizedSAD < localMinWeightedSAD {
						localMinWeightedSAD = normalizedSAD
						localX = x
						localY = y
					}
				}
			}
		}

		if localMinWeightedSAD == math.MaxFloat64 {
			return 0, 0, 0
		}
		// 转换为与原 avgDiff 可比的尺度 (除以3通道)
		return localX, localY, localMinWeightedSAD / 3.0
	}

	if !useConcurrency {
		return matchRect(0, maxX, 0, maxY)
	}

	var mutex sync.Mutex
	globalMinWeightedSAD := math.MaxFloat64
	globalX, globalY := 0, 0
	numWorkers := 8
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	rowsPerWorker := (maxY/step + 1 + numWorkers - 1) / numWorkers

	for i := 0; i < numWorkers; i++ {
		startIdx := i * rowsPerWorker
		endIdx := (i + 1) * rowsPerWorker
		go func(sIdx, eIdx int) {
			defer wg.Done()
			yStart := sIdx * step
			yEnd := eIdx * step
			if yEnd > maxY {
				yEnd = maxY + 1
			}
			lx, ly, lSad := matchRect(0, maxX, yStart, yEnd)
			mutex.Lock()
			if lSad > 0 && lSad < globalMinWeightedSAD {
				globalMinWeightedSAD = lSad
				globalX = lx
				globalY = ly
			}
			mutex.Unlock()
		}(startIdx, endIdx)
	}
	wg.Wait()

	if globalMinWeightedSAD == math.MaxFloat64 {
		return 0, 0, 0
	}
	return globalX, globalY, globalMinWeightedSAD
}

// ==========================================
// 统计置信度评估（三重验证系统 - 策略2）
// ==========================================

// ComputeZScore 计算 Z-Score（rank1 相对于所有结果的标准化距离）
// 返回：rank1 比平均值低多少个标准差（越大越可信）
func ComputeZScore(rank1Score float64, allScores []float64) float64 {
	if len(allScores) < 2 {
		return 0
	}

	// 计算平均值和标准差
	var sum float64
	for _, s := range allScores {
		sum += s
	}
	mean := sum / float64(len(allScores))

	var varianceSum float64
	for _, s := range allScores {
		diff := s - mean
		varianceSum += diff * diff
	}
	stdDev := math.Sqrt(varianceSum / float64(len(allScores)))

	if stdDev < 0.001 {
		return 0 // 所有分数相同
	}

	// Z-score：rank1 比平均值低多少个标准差
	zScore := (mean - rank1Score) / stdDev
	return zScore
}

// ComputeLocalConsistencyFast 快速版本的局部一致性检查
// step: 采样步进，step=4 意味着只用 1/4 的点
func ComputeLocalConsistencyFast(img *image.RGBA, probe *TemplateProbe, matchX, matchY int, step int) float64 {
	imgPix := img.Pix
	imgStride := img.Stride
	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()

	halfW := probe.Width / 2
	halfH := probe.Height / 2

	abs := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}

	var quadScores [4]float64
	var quadCounts [4]int

	// 采样遍历
	for i := 0; i < len(probe.Points); i += step {
		p := &probe.Points[i]

		// 确定点属于哪个象限（简化判断）
		quadIdx := 0
		if p.X >= halfW {
			quadIdx += 1
		}
		if p.Y >= halfH {
			quadIdx += 2
		}

		// 计算该点的颜色差异
		mapX := matchX + p.X
		mapY := matchY + p.Y
		if mapX < 0 || mapX >= imgW || mapY < 0 || mapY >= imgH {
			continue
		}

		off := mapY*imgStride + mapX*4
		if off < 0 || off+2 >= len(imgPix) {
			continue
		}

		r := int(imgPix[off])
		g := int(imgPix[off+1])
		b := int(imgPix[off+2])

		diffR := abs(r - p.R)
		diffG := abs(g - p.G)
		diffB := abs(b - p.B)
		baseDiff := float64(diffR + diffG + diffB)

		quadScores[quadIdx] += baseDiff
		quadCounts[quadIdx]++
	}

	// 计算每个象限的平均分数
	var validQuadAvgs []float64
	for i := 0; i < 4; i++ {
		if quadCounts[i] > 3 { // 降低阈值因为采样更稀疏
			avg := quadScores[i] / float64(quadCounts[i]*3)
			validQuadAvgs = append(validQuadAvgs, avg)
		}
	}

	if len(validQuadAvgs) < 2 {
		return 0
	}

	// 计算象限分数的标准差
	var sum, sumSq float64
	for _, v := range validQuadAvgs {
		sum += v
		sumSq += v * v
	}
	n := float64(len(validQuadAvgs))
	mean := sum / n
	variance := (sumSq / n) - (mean * mean)
	if variance < 0 {
		variance = 0
	}

	return math.Sqrt(variance)
}
