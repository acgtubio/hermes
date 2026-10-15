import cv2, numpy as np, re, subprocess, sys

def osd_rotation(path):
    out = subprocess.run(["tesseract", path, "-", "--psm", "0"], capture_output=True, text=True).stdout
    m = re.search(r"Rotate: (\d+)", out)
    return int(m.group(1)) if m else 0

EDGE_BAND = 0.03  # fraction of the image along each edge where scanner shadows live

def whiten_edges(gray, band=EDGE_BAND):
    """Paint out scanner-edge shadows: dark blobs that touch the image border and stay within a thin
    band along it. A table running off the page isn't one of these, since its blob spans the page."""
    _, bw = cv2.threshold(gray, 0, 255, cv2.THRESH_BINARY_INV + cv2.THRESH_OTSU)
    n, lab, st, _ = cv2.connectedComponentsWithStats(bw)
    H, W = gray.shape
    for i in range(1, n):
        x, y, w, h = st[i, :4]
        if (y == 0 or y + h == H) and h < band * H or (x == 0 or x + w == W) and w < band * W:
            gray[lab == i] = 255
    return gray

def deskew_angle(gray, limit=5.0):
    """Angle that makes text rows and rules horizontal: the rotation maximising the variance of the
    horizontal ink profile (rows of text become sharp peaks). Works with or without ruling lines, on a
    downscaled copy; coarse search, then refine around the best angle."""
    small = cv2.resize(gray, None, fx=1000 / gray.shape[1], fy=1000 / gray.shape[1], interpolation=cv2.INTER_AREA)
    _, bw = cv2.threshold(small, 0, 255, cv2.THRESH_BINARY_INV + cv2.THRESH_OTSU)
    m = int(EDGE_BAND * bw.shape[0])
    bw[:m], bw[-m:] = 0, 0  # shadow lines along the scan edge aren't parallel to the text
    h, w = bw.shape
    def score(a):
        M = cv2.getRotationMatrix2D((w / 2, h / 2), a, 1.0)
        return cv2.warpAffine(bw, M, (w, h), flags=cv2.INTER_NEAREST).sum(1, dtype=np.float64).var()
    best = max(np.arange(-limit, limit + 0.01, 0.25), key=score)
    return float(max(np.arange(best - 0.25, best + 0.26, 0.02), key=score))

def normalize(path):
    img = cv2.imread(path)
    rot = osd_rotation(path)
    codes = {90: cv2.ROTATE_90_CLOCKWISE, 180: cv2.ROTATE_180, 270: cv2.ROTATE_90_COUNTERCLOCKWISE}
    if rot in codes:
        img = cv2.rotate(img, codes[rot])
    gray = whiten_edges(cv2.cvtColor(img, cv2.COLOR_BGR2GRAY))
    ang = deskew_angle(gray)
    h, w = gray.shape
    M = cv2.getRotationMatrix2D((w / 2, h / 2), ang, 1.0)
    gray = cv2.warpAffine(gray, M, (w, h), flags=cv2.INTER_CUBIC, borderValue=255)
    return gray, rot, ang

if __name__ == "__main__":
    g, rot, ang = normalize(sys.argv[1])
    cv2.imwrite("norm.png", g)
    print("rotation", rot, "deskew deg", round(ang, 3), "size", g.shape)
