import cv2, numpy as np, re, subprocess, sys

def osd_rotation(path):
    out = subprocess.run(["tesseract", path, "-", "--psm", "0"], capture_output=True, text=True).stdout
    m = re.search(r"Rotate: (\d+)", out)
    return int(m.group(1)) if m else 0

def deskew_angle(gray):
    bw = cv2.adaptiveThreshold(gray, 255, cv2.ADAPTIVE_THRESH_MEAN_C, cv2.THRESH_BINARY_INV, 25, 15)
    horiz = cv2.morphologyEx(bw, cv2.MORPH_OPEN, cv2.getStructuringElement(cv2.MORPH_RECT, (gray.shape[1] // 8, 1)))
    lines = cv2.HoughLinesP(horiz, 1, np.pi / 1800, 200, minLineLength=gray.shape[1] // 4, maxLineGap=20)
    if lines is None:
        return 0.0
    angs = [np.degrees(np.arctan2(y2 - y1, x2 - x1)) for x1, y1, x2, y2 in lines[:, 0]]
    return float(np.median(angs))

def normalize(path):
    img = cv2.imread(path)
    rot = osd_rotation(path)
    codes = {90: cv2.ROTATE_90_CLOCKWISE, 180: cv2.ROTATE_180, 270: cv2.ROTATE_90_COUNTERCLOCKWISE}
    if rot in codes:
        img = cv2.rotate(img, codes[rot])
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    ang = deskew_angle(gray)
    h, w = gray.shape
    M = cv2.getRotationMatrix2D((w / 2, h / 2), ang, 1.0)
    gray = cv2.warpAffine(gray, M, (w, h), flags=cv2.INTER_CUBIC, borderValue=255)
    return gray, rot, ang

if __name__ == "__main__":
    g, rot, ang = normalize(sys.argv[1])
    cv2.imwrite("norm.png", g)
    print("rotation", rot, "deskew deg", round(ang, 3), "size", g.shape)
