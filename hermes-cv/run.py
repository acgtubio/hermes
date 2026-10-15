"""Whole pipeline over a PDF or a single scan: python run.py <doc.pdf|scan.png> [outdir]
Writes each stage's files to <outdir>/page-NN/ (same names as the single-page scripts), validates
totals across pages, and writes the whole document to <outdir>/document.json."""
import cv2, json, os, sys, time
from concurrent.futures import ProcessPoolExecutor
import ingest, normalize, ocr_body, reread, rows, validate

def run_page(d):
    g, rot, ang = normalize.normalize(os.path.join(d, "page.png"))
    cv2.imwrite(os.path.join(d, "norm.png"), g)
    table = ocr_body.find_table(g)
    if table is None:
        return d, None, "no ruled table"
    json.dump(table, open(os.path.join(d, "table.json"), "w"))
    df = ocr_body.ocr(ocr_body.clean(ocr_body.body_of(g, table)))
    df.to_csv(os.path.join(d, "words.csv"), index=False)
    out = rows.parse(df, table["w"])
    if not out["rows"]:
        return d, None, "no table rows"
    fixes = reread.repair(g, table, out)
    json.dump(out, open(os.path.join(d, "rows.json"), "w"), indent=1)
    return d, out, f"rotation {rot}, deskew {ang:.2f} deg, {len(out['rows'])} rows, {len(out['subtotals'])} subtotals, {len(fixes)} cells repaired" + "".join(
        f"\n    {where} {k}: {old!r} -> {new!r}" for where, k, old, new in fixes)

def process(src, outdir, log=print):
    """Run every page of a PDF (or one scan) through the pipeline and validate totals across pages.
    Returns the document: {page_count, pages: [{page, rows, subtotals, footer}], flags, totals}, where
    pages are the table pages, in rows.json's shapes. Also writes it to <outdir>/document.json."""
    images = ingest.page_images(src) if src.lower().endswith(".pdf") else [cv2.imread(src, 0)]
    dirs = []
    for n, img in enumerate(images, 1):
        d = os.path.join(outdir, f"page-{n:02d}"); os.makedirs(d, exist_ok=True)
        cv2.imwrite(os.path.join(d, "page.png"), img); dirs.append(d)
    t = time.time()
    with ProcessPoolExecutor() as ex:
        results = list(ex.map(run_page, dirs))
    pages = []
    for n, (d, out, note) in enumerate(results, 1):
        log(os.path.basename(d), "-", note if out else f"skipped ({note})")
        if out: pages.append((n, os.path.basename(d), d, out))
    log(f"{len(pages)} table pages, {sum(len(o['rows']) for *_, o in pages)} rows in {time.time() - t:.0f}s\n")
    flags = [{"page": n, "row": i, "message": m} for n, _, _, o in pages for i, m in validate.check_rows(o)]
    for f in flags: log(f"page-{f['page']:02d} row {f['row']} - {f['message']}")
    totals = validate.check_totals([(name, o) for _, name, _, o in pages])
    log("\n".join(totals))
    for _, _, d, out in pages:  # check_totals set the subtotal and footer cells' status
        json.dump(out, open(os.path.join(d, "rows.json"), "w"), indent=1)
    doc = {"page_count": len(results), "pages": [{"page": n, **o} for n, _, _, o in pages], "flags": flags, "totals": totals}
    json.dump(doc, open(os.path.join(outdir, "document.json"), "w"), indent=1)
    return doc

if __name__ == "__main__":
    process(sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else "out")
