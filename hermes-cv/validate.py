import json, re
from decimal import Decimal as D
d = json.load(open("rows.json"))
num = lambda s: D(s.replace(",", ""))
flags, tot = [], {k: D(0) for k in ["sales","vat","total_sales_w_vat","withholding_tax","net_amount"]}
for r in d["rows"]:
    c = {k: v["text"] for k, v in r["cells"].items()}; i = r["row_index"]
    if not re.fullmatch(r"\d{2}\.\d{2}\.\d{4}", c["doc_date"]): flags.append((i, "date format"))
    for k in ["po_number", "service_entry_sheet"]:
        if not re.fullmatch(r"\d{10}", c[k]): flags.append((i, k + " not 10 digits"))
    s, v, t, w, n = (num(c[k]) for k in tot)
    for k, x in zip(tot, (s, v, t, w, n)): tot[k] += x
    if s + v != t: flags.append((i, "sales + vat != total"))
    if t - w != n: flags.append((i, "total - withholding != net"))
    if (s * D("0.02")).quantize(D("0.01")) != w: flags.append((i, f"withholding != 2% of sales ({s*D('0.02')})"))
    low = [k for k, v in r["cells"].items() if v["conf"] < 80]
    if low: flags.append((i, "low confidence: " + ", ".join(low)))
for k, v in tot.items():
    f = num(d["footer"][k])
    print(f"{k:20s} sum {v:>10}  footer {f:>10}  {'OK' if v == f else 'MISMATCH'}")
print("\nflags:"); [print(" row", i, "-", m) for i, m in flags]
