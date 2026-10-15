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

def add(a, b): return [x + y for x, y in zip(a, b)]
def sub(a, b): return [x - y for x, y in zip(a, b)]
def diff(label, got, want):
    return f"{label}: " + ", ".join(f"{k} {g} vs {w}" for k, g, w in zip(AMOUNTS, got, want) if g != w)

EARLIER = " (expected if earlier pages aren't included: pass every page's rows.json in order)"

def check_totals(pages):
    """Each page's NET AMOUNT footer is a running total over the document (the last one is the grand total),
    and subtotals close groups of rows that can span pages. A footer passes if it equals the running total,
    the page's own total (a single-page document), or the change from the previous footer (so an OCR error
    on one page doesn't fail every page after it)."""
    msgs, cum, group, prev, first_group = [], [D(0)] * 5, [D(0)] * 5, None, True
    for name, d in pages:
        page, subs = [D(0)] * 5, d.get("subtotals", [])
        for k in range(len(d["rows"]) + 1):
            if k:
                a = row_amounts(d["rows"][k - 1])
                if a: page, group = add(page, a), add(group, a)
            for s in (s for s in subs if s["after_row"] == k):
                st = [num(s["cells"].get(f, {}).get("text")) for f in AMOUNTS]
                if None in st: msgs.append(f"{name}: subtotal after row {k} unreadable")
                elif st != group: msgs.append(f"{name}: " + diff(f"subtotal after row {k} MISMATCH", group, st)
                                              + (EARLIER if first_group else ""))
                group, first_group = [D(0)] * 5, False
        cum = add(cum, page)
        footer = {k: c["text"] for k, c in (d.get("footer") or {}).items()}
        f = [num(footer.get(k)) for k in AMOUNTS]
        if None in f:
            msgs.append(f"{name}: footer missing or unreadable {footer}"); prev = None; continue
        if f == cum: msgs.append(f"{name}: footer OK (running total {f[0]})")
        elif f == page: msgs.append(f"{name}: footer OK (page total {f[0]})")
        elif prev is not None and sub(f, prev) == page: msgs.append(f"{name}: footer OK vs previous page; an earlier page is off")
        elif prev: msgs.append(f"{name}: " + diff("footer MISMATCH, page sum vs footer change", page, sub(f, prev)))
        else: msgs.append(f"{name}: " + diff("footer MISMATCH, running total vs footer", cum, f) + EARLIER)
        prev = f
    return msgs

if __name__ == "__main__":
    paths = sys.argv[1:] or ["rows.json"]  # several pages' rows.json, in page order, for cross-page totals
    pages = [(p, json.load(open(p))) for p in paths]
    for p, d in pages:
        for i, m in check_rows(d): print(f"{p} row {i} - {m}")
    print("\n".join(check_totals(pages)))
