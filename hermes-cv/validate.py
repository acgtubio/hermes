import json, re, sys
from decimal import Decimal as D, ROUND_HALF_UP

# Field rules of the SOA report
FORMATS = {"doc_date": r"\d{2}\.\d{2}\.\d{4}", "po_number": r"\d{10}", "service_entry_sheet": r"\d{10}",
           "reference_no": r"B?\d{6}[A-Z]?(-[A-Z]{1,4})?", "doc_header_text": r"\d{4}"}
AMOUNTS = ["sales", "vat", "total_sales_w_vat", "withholding_tax", "net_amount"]
WITHHOLDING_RATE = D("0.02")
MIN_CONF = 80

def num(s):
    """Amount text as a Decimal, or None if it isn't a well-formed amount (e.g. an OCR slip like 1.113.00).
    The report groups only the last thousands: 1001,579.43."""
    s = (s or "").strip()
    return D(s.replace(",", "")) if re.fullmatch(r"\d+(,\d{3})?\.\d{2}", s) else None

def row_amounts(r):
    a = [num(r["cells"].get(k, {}).get("text")) for k in AMOUNTS]
    return None if None in a else a

def arithmetic(a):
    s, v, t, w, n = a
    wt = (s * WITHHOLDING_RATE).quantize(D("0.01"), ROUND_HALF_UP)
    return ([] if s + v == t else ["sales + vat != total"]) + ([] if t - w == n else ["total - withholding != net"]) + \
           ([] if wt == w else [f"withholding != 2% of sales ({wt})"])

def row_ok(r):
    a = row_amounts(r)
    return a is not None and not arithmetic(a)

def fmt(d):
    """Decimal as the report prints it, grouping only the last thousands: 1001,579.43."""
    i, f = f"{d:.2f}".split(".")
    return (f"{i[:-3]},{i[-3:]}" if len(i) > 3 else i) + "." + f

# Each amount recomputed from the others, in AMOUNTS order (sales, vat, total, withholding, net)
DERIVE = [lambda s, v, t, w, n: t - v, lambda s, v, t, w, n: t - s, lambda s, v, t, w, n: s + v,
          lambda s, v, t, w, n: t - n, lambda s, v, t, w, n: t - w]

def infer(r):
    """The one amount that, recomputed from the row's other four, makes the arithmetic hold: (field, Decimal).
    Covers unreadable cells (4217) and misreads that parse (1,639.00 for 1,539.00). None when no single
    cell fixes the row, or when more than one would (a row that already holds, or an ambiguous one)."""
    a = [num(r["cells"].get(k, {}).get("text")) for k in AMOUNTS]
    found = []
    for j, derive in enumerate(DERIVE):
        b = a[:j] + [None] + a[j + 1:]
        try: c = derive(*b)
        except TypeError: continue  # a cell it's derived from is unreadable too
        b[j] = c
        if None not in b and c >= 0 and not arithmetic(b): found.append((AMOUNTS[j], c))
    return found[0] if len(found) == 1 else None

def cell_status(k, c, amounts_ok):
    """ok / repaired / inferred: passed its field's check (repaired and inferred cells keep `first_read`);
    failed: still doesn't; unchecked: the field has no check."""
    if k in AMOUNTS: good = amounts_ok
    elif k in FORMATS: good = bool(re.fullmatch(FORMATS[k], c["text"]))
    else: return "unchecked"
    if not good: return "failed"
    return "inferred" if c.get("status") == "inferred" else "repaired" if "first_read" in c else "ok"

def annotate_rows(d):
    for r in d["rows"]:
        ok = row_ok(r)
        for k, c in r["cells"].items(): c["status"] = cell_status(k, c, ok)

def check_rows(d):
    flags = []
    for r in d["rows"]:
        c = {k: v["text"] for k, v in r["cells"].items()}; i = r["row_index"]
        for k, pat in FORMATS.items():
            if not re.fullmatch(pat, c.get(k, "")): flags.append((i, f"{k} format: {c.get(k, '')!r}"))
        a = row_amounts(r)
        if a is None:
            flags.append((i, "unreadable amount: " + ", ".join(k for k in AMOUNTS if num(c.get(k)) is None)))
        else:
            flags += [(i, m) for m in arithmetic(a)]
        low = [k for k, v in r["cells"].items() if v["conf"] < MIN_CONF]
        if low: flags.append((i, "low confidence: " + ", ".join(low)))
    return flags

def add(a, b): return [x + (y or 0) for x, y in zip(a, b)]  # an unreadable cell drops out of its column only
def sub(a, b): return [x - y for x, y in zip(a, b)]
def diff(label, got, want, unreadable=0):
    return f"{label}: " + ", ".join(f"{k} {g} vs {w}" for k, g, w in zip(AMOUNTS, got, want) if g != w) + \
           (f" ({unreadable} unreadable row cells left out)" if unreadable else "")

def mark(cells, ok):
    for k, good in zip(AMOUNTS, ok):
        if k in cells: cells[k]["status"] = "ok" if good else "failed"

EARLIER = " (expected if earlier pages aren't included: pass every page's rows.json in order)"

def check_totals(pages):
    """Each page's NET AMOUNT footer is a running total over the document (the last one is the grand total),
    and subtotals close groups of rows that can span pages. A footer passes if it equals the running total,
    the page's own total (a single-page document), or the change from the previous footer (so an OCR error
    on one page doesn't fail every page after it). Sums are per column, so an unreadable row cell only leaves
    its own column short. Sets `status` (ok/failed) on each subtotal and footer cell."""
    msgs, cum, group, prev, first_group = [], [D(0)] * 5, [D(0)] * 5, None, True
    cum_bad = group_bad = 0
    for name, d in pages:
        page, subs, page_bad = [D(0)] * 5, d.get("subtotals", []), 0
        for k in range(len(d["rows"]) + 1):
            if k:
                a = [num(d["rows"][k - 1]["cells"].get(f, {}).get("text")) for f in AMOUNTS]
                bad = a.count(None)
                page, group, page_bad, group_bad = add(page, a), add(group, a), page_bad + bad, group_bad + bad
            for s in (s for s in subs if s["after_row"] == k):
                st = [num(s["cells"].get(f, {}).get("text")) for f in AMOUNTS]
                mark(s["cells"], [x is not None and x == g for x, g in zip(st, group)])
                if None in st: msgs.append(f"{name}: subtotal after row {k} unreadable")
                elif st != group: msgs.append(f"{name}: " + diff(f"subtotal after row {k} MISMATCH", group, st, group_bad)
                                              + (EARLIER if first_group else ""))
                group, group_bad, first_group = [D(0)] * 5, 0, False
        cum, cum_bad = add(cum, page), cum_bad + page_bad
        cells = d.get("footer") or {}
        f = [num(cells.get(k, {}).get("text")) for k in AMOUNTS]
        q = prev or [None] * 5
        mark(cells, [x is not None and (x == c or x == p or (y is not None and x - y == p)) for x, c, p, y in zip(f, cum, page, q)])
        if None in f:
            msgs.append(f"{name}: footer missing or unreadable {({k: c['text'] for k, c in cells.items()})}"); prev = None; continue
        if f == cum: msgs.append(f"{name}: footer OK (running total {f[0]})")
        elif f == page: msgs.append(f"{name}: footer OK (page total {f[0]})")
        elif prev is not None and sub(f, prev) == page: msgs.append(f"{name}: footer OK vs previous page; an earlier page is off")
        elif prev: msgs.append(f"{name}: " + diff("footer MISMATCH, page sum vs footer change", page, sub(f, prev), page_bad))
        else: msgs.append(f"{name}: " + diff("footer MISMATCH, running total vs footer", cum, f, cum_bad) + EARLIER)
        prev = f
    return msgs

if __name__ == "__main__":
    paths = sys.argv[1:] or ["rows.json"]  # several pages' rows.json, in page order, for cross-page totals
    pages = [(p, json.load(open(p))) for p in paths]
    for p, d in pages:
        for i, m in check_rows(d): print(f"{p} row {i} - {m}")
    print("\n".join(check_totals(pages)))
