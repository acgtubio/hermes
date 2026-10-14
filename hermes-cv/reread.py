import json, cv2, pytesseract, time
g = cv2.imread("norm.png", 0); X0, BODY_TOP = 380, 566
d = json.load(open("rows.json"))
wl = {"po_number": "0123456789", "service_entry_sheet": "0123456789", "doc_header_text": "0123456789",
      "reference_no": "B0123456789ABCD", "doc_date": "0123456789."}
t = time.time(); agree = disagree = 0
for r in d["rows"]:
    for k, chars in wl.items():
        c = r["cells"][k]; x0, y0, x1, y1 = c["bbox"]
        crop = g[BODY_TOP + y0 - 8: BODY_TOP + y1 + 8, X0 + x0 - 8: X0 + x1 + 8]
        crop = cv2.resize(crop, None, fx=1.5, fy=1.5, interpolation=cv2.INTER_CUBIC)
        txt = pytesseract.image_to_string(crop, config=f"--psm 7 -c tessedit_char_whitelist={chars}").strip()
        if txt == c["text"]: agree += 1
        else: disagree += 1; print("row", r["row_index"], k, "first:", c["text"], "re-read:", txt)
print(f"{agree} agree, {disagree} disagree, {time.time()-t:.1f}s for {agree+disagree} cells")
