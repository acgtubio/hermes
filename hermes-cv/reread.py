import json, re, cv2, pytesseract, time
from validate import FORMATS, AMOUNTS, annotate_rows, fmt, infer, num, row_ok

AMOUNT_CHARS = "0123456789.,"
WHITELISTS = {"po_number": "0123456789", "service_entry_sheet": "0123456789", "doc_header_text": "0123456789",
              "reference_no": "B0123456789ABCDIM-", "doc_date": "0123456789.", **{k: AMOUNT_CHARS for k in AMOUNTS}}

def reread(g, table, c, field):
    """Re-OCR one cell from its bbox (relative to the table crop) as a single line, allowing only the
    characters the field can contain."""
    x0, y0, x1, y1 = c["bbox"]
    crop = g[max(0, table["y"] + y0 - 8): table["y"] + y1 + 8, max(0, table["x"] + x0 - 8): table["x"] + x1 + 8]
    crop = cv2.resize(crop, None, fx=1.5, fy=1.5, interpolation=cv2.INTER_CUBIC)
    return pytesseract.image_to_string(crop, config=f"--psm 7 -c tessedit_char_whitelist={WHITELISTS[field]}").strip()

def repair(g, table, d):
    """Second pass over cells that fail a check: keep a re-read only when it passes the check the first
    read failed (format for ID fields, the row's arithmetic for amounts). If no re-read fixes a row's
    amounts, the one cell its arithmetic determines is inferred. Changed cells keep `first_read`; every row
    cell gets a `status` (see validate.cell_status)."""
    fixes = []
    for r in d["rows"]:
        cells = r["cells"]
        for k, pat in FORMATS.items():
            if k in cells and not re.fullmatch(pat, cells[k]["text"]):
                tokens = [w for w in cells[k]["text"].split() if re.fullmatch(pat, w)]
                # one valid token beside stray marks (a handwritten tick read as "4"), else re-OCR
                txt = tokens[0] if len(tokens) == 1 else reread(g, table, cells[k], k)
                if re.fullmatch(pat, txt):
                    fixes.append((f"row {r['row_index']}", k, cells[k]["text"], txt))
                    cells[k]["first_read"], cells[k]["text"] = cells[k]["text"], txt
        if not row_ok(r):
            for k in (k for k in AMOUNTS if k in cells):
                old = cells[k]["text"]; cells[k]["text"] = reread(g, table, cells[k], k)
                if row_ok(r):
                    fixes.append((f"row {r['row_index']}", k, old, cells[k]["text"])); cells[k]["first_read"] = old
                    break
                cells[k]["text"] = old
        if not row_ok(r) and (found := infer(r)):
            k, v = found
            c = cells.setdefault(k, {"text": "", "conf": 0.0, "bbox": None})  # a cell OCR missed altogether
            fixes.append((f"row {r['row_index']}", k, c["text"], fmt(v) + " (inferred)"))
            c["first_read"], c["text"], c["status"] = c["text"], fmt(v), "inferred"
    for label, cells in [(f"subtotal after row {s['after_row']}", s["cells"]) for s in d["subtotals"]] + [("footer", d["footer"] or {})]:
        for k, c in cells.items():  # totals are checked across pages later; here just make them readable
            if num(c["text"]) is None:
                txt = reread(g, table, c, k)
                if num(txt) is not None:
                    fixes.append((label, k, c["text"], txt)); c["first_read"], c["text"] = c["text"], txt
    annotate_rows(d)
    return fixes

if __name__ == "__main__":
    g = cv2.imread("norm.png", 0); table = json.load(open("table.json"))
    d = json.load(open("rows.json"))
    t = time.time()
    for where, k, old, new in repair(g, table, d): print(where, k, "first:", old, "re-read:", new)
    json.dump(d, open("rows.json", "w"), indent=1)
    print(f"{time.time() - t:.1f}s")
