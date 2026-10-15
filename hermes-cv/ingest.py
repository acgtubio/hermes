import fitz, numpy as np, os, sys, cv2

def page_images(path):
    """Yield one grayscale image per PDF page, upright as displayed (the page's /Rotate is applied).
    Scanned pages are rendered at the embedded scan's own resolution, so pixels aren't resampled."""
    doc = fitz.open(path)
    for page in doc:
        info = page.get_image_info()
        longest = max((max(i["width"], i["height"]) for i in info), default=0)
        dpi = max(300, round(72 * longest / max(page.rect.width, page.rect.height)))
        pix = page.get_pixmap(dpi=dpi, colorspace=fitz.csGRAY)
        yield np.frombuffer(pix.samples, np.uint8).reshape(pix.height, pix.width)

if __name__ == "__main__":
    out = sys.argv[2] if len(sys.argv) > 2 else "."
    os.makedirs(out, exist_ok=True)
    for n, img in enumerate(page_images(sys.argv[1]), 1):
        cv2.imwrite(os.path.join(out, f"page-{n:02d}.png"), img)
        print(f"page-{n:02d}.png", img.shape)
