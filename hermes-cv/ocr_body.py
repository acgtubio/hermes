import cv2, pytesseract, pandas as pd, time
g = cv2.imread("norm.png", 0)
X0, X1, HDR_TOP, BODY_TOP = 380, 3384, 390, 566
body = g[BODY_TOP:, X0:X1]
t = time.time()
df = pytesseract.image_to_data(body, config="--psm 6", output_type=pytesseract.Output.DATAFRAME)
print("ocr seconds", round(time.time() - t, 2))
df = df[(df.conf > -1) & df.text.notna()]
df["text"] = df.text.astype(str).str.strip()
df = df[df.text != ""]
df["cx"] = df.left + df.width / 2
df["cy"] = df.top + df.height / 2
df.to_csv("words.csv", index=False)
print(len(df), "words; median height", df.height.median())
print(df[["left","top","width","height","conf","text"]].head(40).to_string())
