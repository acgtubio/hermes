import cv2, json, numpy as np, pandas as pd, pytesseract, time
from normalize import EDGE_BAND

INSET = 8  # px inside the outer rules, so the borders themselves aren't OCR'd

def find_table(gray):
    """Table box from its long horizontal rules (top border, header rule, bottom border if on the page):
    x-extent is the union of the rules, top is the highest rule; the body runs to the image bottom
    (the scan often cuts the table off). Returns None when the page has no ruled table."""
    bw = cv2.adaptiveThreshold(gray, 255, cv2.ADAPTIVE_THRESH_MEAN_C, cv2.THRESH_BINARY_INV, 25, 15)
    k = gray.shape[1] // 80  # short, so a rule on a warped page survives as steps
    h = cv2.morphologyEx(bw, cv2.MORPH_OPEN, cv2.getStructuringElement(cv2.MORPH_RECT, (k, 1)))
    h = cv2.morphologyEx(h, cv2.MORPH_CLOSE, cv2.getStructuringElement(cv2.MORPH_RECT, (k // 3, 3)))  # rejoin steps and faint breaks
    _, _, st, _ = cv2.connectedComponentsWithStats(h)
    H, m = gray.shape[0], EDGE_BAND * gray.shape[0]
    top, height = st[:, cv2.CC_STAT_TOP], st[:, cv2.CC_STAT_HEIGHT]
    keep = (st[:, cv2.CC_STAT_WIDTH] > gray.shape[1] // 2) & (top > m) & (top + height < H - m)  # skip edge shadows
    rules = st[1:][keep[1:]]
    if not len(rules):
        return None
    x0, x1 = rules[:, cv2.CC_STAT_LEFT].min(), (rules[:, cv2.CC_STAT_LEFT] + rules[:, cv2.CC_STAT_WIDTH]).max()
    top = rules[:, cv2.CC_STAT_TOP].min()
    return {"x": int(x0) + INSET, "y": int(top) + INSET, "w": int(x1 - x0) - 2 * INSET}

def clean(gray):
    """Flatten the background (divide by a local max) and whiten what's lighter than halfway between
    ink and paper: removes scanner streaks that Tesseract otherwise reads as `~`, `_`, `|`.
    Stays grayscale on purpose: binarizing hurt accuracy, and despeckling deletes decimal points."""
    k = gray.shape[1] // 100 | 1
    bg = cv2.morphologyEx(gray, cv2.MORPH_CLOSE, cv2.getStructuringElement(cv2.MORPH_RECT, (k, k)))
    flat = cv2.divide(gray, bg, scale=255)
    t, _ = cv2.threshold(flat, 0, 255, cv2.THRESH_BINARY + cv2.THRESH_OTSU)
    flat[flat > (t + 255) / 2] = 255
    return flat

def ocr(body, psm=6):
    df = pytesseract.image_to_data(body, config=f"--psm {psm}", output_type=pytesseract.Output.DATAFRAME)
    df = df[(df.conf > -1) & df.text.notna()].copy()
    df["text"] = df.text.astype(str).str.strip()
    df = df[df.text != ""]
    df = df[~(df.text.str.fullmatch(r"[\W_]+") & (df.conf < 60))]  # low-confidence punctuation is scan noise
    df["cx"] = df.left + df.width / 2
    df["cy"] = df.top + df.height / 2
    return df

def body_of(gray, table):
    return gray[table["y"]:, table["x"]:table["x"] + table["w"]]

if __name__ == "__main__":
    g = cv2.imread("norm.png", 0)
    table = find_table(g)
    if table is None:
        raise SystemExit("no ruled table found")
    json.dump(table, open("table.json", "w"))
    t = time.time()
    df = ocr(clean(body_of(g, table)))
    print("table", table, "ocr seconds", round(time.time() - t, 2))
    df.to_csv("words.csv", index=False)
    print(len(df), "words; median height", df.height.median())
    print(df[["left","top","width","height","conf","text"]].head(40).to_string())
