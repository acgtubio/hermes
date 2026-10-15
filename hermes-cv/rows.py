import pandas as pd, re, json

COLUMNS = ["doc_date","po_number","service_entry_sheet","reference_no","doc_header_text","text","sales","vat","total_sales_w_vat","withholding_tax","net_amount"]
AMOUNTS = COLUMNS[6:]
# Boundaries between columns as fractions of the ruled table width. The SOA layout is fixed, so these come from
# the template (median whitespace gaps over the Sep 1-15 2026 sample, which varied by < 0.01 and also hold on the
# Jun and Jul samples) instead of being re-detected per page, where stray marks or signatures bridge or hide gaps.
EDGES = [0.067, 0.144, 0.226, 0.318, 0.378, 0.578, 0.682, 0.746, 0.830, 0.917]
ID_FIELDS = ["po_number", "service_entry_sheet", "reference_no", "doc_header_text"]
DATE = re.compile(r"^\d{2}\.\d{2}\.\d{4}$")
AMOUNT = re.compile(r"\d\.\d{2}$")

def text_lines(df):
    """Words grouped into lines by Tesseract's own line segmentation, which follows a row across the page
    even when the paper was warped and the row drifts by more than its height."""
    lines = [g.sort_values("left") for _, g in df.groupby(["block_num", "par_num", "line_num"])]
    return [list(g.itertuples()) for g in sorted(lines, key=lambda g: g.cy.median())]

def cell(ws):
    return {"text": " ".join(w.text for w in ws), "conf": round(float(min(w.conf for w in ws)), 1),
            "bbox": [int(min(w.left for w in ws)), int(min(w.top for w in ws)),
                     int(max(w.left + w.width for w in ws)), int(max(w.top + w.height for w in ws))]}

def extend(c, ws):
    """Append a continuation line's words to a cell."""
    more = cell(ws)
    c["text"] = (c["text"] + " " + more["text"]).strip()
    c["conf"] = min(c["conf"], more["conf"])
    b, m = c["bbox"], more["bbox"]
    c["bbox"] = [min(b[0], m[0]), min(b[1], m[1]), max(b[2], m[2]), max(b[3], m[3])]

def parse(df, width):
    """Rows start on a line whose first word is a date in the first column; other lines continue the
    previous row. A line with amounts but no date is a totals line: the last one on the page is the
    NET AMOUNT footer, earlier ones are subtotals. Anything after the footer (signatures) is ignored."""
    bounds = [e * width for e in EDGES]
    col = lambda cx: COLUMNS[sum(cx >= b for b in bounds)]
    lines = []
    for ln in text_lines(df):
        cells = {}
        for w in ln:
            cells.setdefault(col(w.cx), []).append(w)
        is_row = bool(DATE.match(ln[0].text)) and col(ln[0].cx) == COLUMNS[0]
        amounts = sum(bool(AMOUNT.search(" ".join(w.text for w in cells.get(k, [])))) for k in AMOUNTS)
        is_total = not is_row and amounts >= 3  # wrapped text and column headers never carry amounts
        lines.append((cells, is_row, is_total))
    last_total = max((i for i, (_, _, t) in enumerate(lines) if t), default=None)
    rows, subtotals, footer = [], [], None
    for i, (cells, is_row, is_total) in enumerate(lines):
        if is_total:
            totals = {k: cell(cells[k]) for k in AMOUNTS if k in cells}
            if i == last_total:
                footer = totals
                break
            subtotals.append({"after_row": len(rows), "cells": totals})
        elif is_row:
            rows.append({k: cell(ws) for k, ws in cells.items()})
        elif rows:  # lines before the first row are the column headers
            for k, ws in cells.items():
                if k in rows[-1]:
                    extend(rows[-1][k], ws)
                else:
                    rows[-1][k] = cell(ws)
    for r in rows:
        for k in ID_FIELDS:
            if k in r:  # specks next to an ID read as leading/trailing punctuation, e.g. "2771."
                r[k]["text"] = re.sub(r"^\W+|\W+$", "", r[k]["text"])
    return {"rows": [dict(row_index=i + 1, cells=r) for i, r in enumerate(rows)], "subtotals": subtotals, "footer": footer}

if __name__ == "__main__":
    df = pd.read_csv("words.csv", dtype={"text": str})
    out = parse(df, json.load(open("table.json"))["w"])
    json.dump(out, open("rows.json", "w"), indent=1)
    print(len(out["rows"]), "rows;", len(out["subtotals"]), "subtotals; footer:", out["footer"])
    for r in out["rows"]:
        print(r["row_index"], " | ".join(r["cells"].get(n, {}).get("text", "") for n in COLUMNS))
