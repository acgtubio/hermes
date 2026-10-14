import pandas as pd, numpy as np, re, json
df = pd.read_csv("words.csv", dtype={"text": str})
W = 3004
# Column boundaries: x ranges with no word coverage across the whole body
cov = np.zeros(W, bool)
for _, r in df.iterrows():
    cov[int(r.left):int(r.left + r.width)] = True
gaps, start = [], None
for x in range(W):
    if not cov[x] and start is None: start = x
    if cov[x] and start is not None:
        if x - start >= 25: gaps.append((start + x) // 2)
        start = None
names = ["doc_date","po_number","service_entry_sheet","reference_no","doc_header_text","text","sales","vat","total_sales_w_vat","withholding_tax","net_amount"]
print("gaps found", len(gaps), gaps)
bounds = [0] + gaps + [W]
def col_of(cx):
    for i in range(len(bounds) - 1):
        if bounds[i] <= cx < bounds[i + 1]: return i
# Text lines: cluster words by vertical centre
df = df.sort_values("cy")
lines, cur = [], []
for _, r in df.iterrows():
    if cur and r.cy - np.mean([w.cy for w in cur]) > 12:
        lines.append(cur); cur = []
    cur.append(r)
lines.append(cur)
DATE = re.compile(r"^\d{2}\.\d{2}\.\d{4}$")
records, footer = [], None
for ln in lines:
    ln = sorted(ln, key=lambda w: w.left)
    cells = {}
    for w in ln:
        c = col_of(w.cx)
        cells.setdefault(c, []).append(w)
    first = ln[0].text
    joined = " ".join(w.text for w in ln)
    if "NET" in joined and "AMOUNT" in joined:
        footer = {names[c]: " ".join(w.text for w in ws) for c, ws in cells.items() if c >= 6}
        continue
    if DATE.match(first) and col_of(ln[0].cx) == 0:
        rec = {names[c]: {"text": " ".join(w.text for w in ws), "conf": round(min(w.conf for w in ws), 1),
               "bbox": [int(min(w.left for w in ws)), int(min(w.top for w in ws)),
                        int(max(w.left + w.width for w in ws)), int(max(w.top + w.height for w in ws))]}
               for c, ws in cells.items()}
        records.append(rec)
    elif records:  # continuation line: append to its column in the previous record
        for c, ws in cells.items():
            cell = records[-1].setdefault(names[c], {"text": "", "conf": 100.0, "bbox": None})
            cell["text"] = (cell["text"] + " " + " ".join(w.text for w in ws)).strip()
            cell["conf"] = round(min(cell["conf"], min(w.conf for w in ws)), 1)
out = {"rows": [dict(row_index=i + 1, cells=r) for i, r in enumerate(records)], "footer": footer}
json.dump(out, open("rows.json", "w"), indent=1)
print(len(records), "records; footer:", footer)
for i, r in enumerate(records, 1):
    print(i, " | ".join(r.get(n, {}).get("text", "") for n in names))
