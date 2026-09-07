#!/usr/bin/env python3
"""Generate the FTEST document corpus.

    python scripts/gencorpus/gencorpus.py <output-directory>

The corpus is the set of documents the FTEST passes sign, render and fuzz
against: every page count, page size, rotation, cross-reference mechanism,
font arrangement and image codec this project has a code path for, plus the
shapes it deliberately degrades rather than draws.

It exists as a script rather than as bytes in the repository for the reason
D-071 gives for testdata/pdfs/blank.pdf: a fixture nobody can regenerate is
a fixture that drifts.  The FTEST Group 1/2 pass built this corpus in a
scratch directory and deleted it with the rest of the session's scratch, so
Group 3 had to rebuild it from the report's own description.  Committing the
generator is what stops that happening a third time.

Requires reportlab and Pillow, which are what the original corpus was built
with.  Nothing in the product depends on either; this is a developer tool.
"""

import os
import struct
import sys
import zlib

from io import BytesIO

from PIL import Image, ImageDraw
from reportlab.lib.pagesizes import A3, A4, A5, LETTER
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.pdfgen import canvas

CYRILLIC = "Уговор о пружању услуга. Странка: ВЕЉКО СТАНОЈЕВИЋ."
LATIN = "Ugovor o pruzanju usluga. Stranka: Zoran Milovanovic."
MIXED = "Račun / Рачун br. 2026-0417 — čćđšž ЧЋЂШЖ"


def find_embeddable_font():
    """A real TrueType file to embed, so /FontFile2 has a code path."""
    for path in (
        r"C:\Windows\Fonts\arial.ttf",
        r"C:\Windows\Fonts\segoeui.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
    ):
        if os.path.exists(path):
            return path
    return None


def page_text(c, text, y=760, font="Helvetica", size=11):
    c.setFont(font, size)
    c.drawString(60, y, text)


# ---------------------------------------------------------------- reportlab


def doc_pages(path, n, size=A4, text=LATIN, rotate=None, font="Helvetica"):
    c = canvas.Canvas(path, pagesize=size)
    for i in range(n):
        if rotate is not None:
            c.setPageRotation(rotate if not isinstance(rotate, list) else rotate[i % len(rotate)])
        page_text(c, f"{text}  [page {i + 1} of {n}]", y=size[1] - 80, font=font)
        c.setStrokeColorRGB(0, 0, 0)
        c.rect(50, 50, size[0] - 100, size[1] - 140)
        c.showPage()
    c.save()


def doc_producer(path, producer):
    c = canvas.Canvas(path, pagesize=A4)
    c.setProducer(producer)
    c.setTitle("Ugovor")
    page_text(c, f"Produced by {producer}")
    c.showPage()
    c.save()


def doc_embedded_font(path, ttf):
    pdfmetrics.registerFont(TTFont("Embedded", ttf))
    c = canvas.Canvas(path, pagesize=A4)
    c.setFont("Embedded", 13)
    c.drawString(60, 760, CYRILLIC)
    c.drawString(60, 735, MIXED)
    c.showPage()
    c.save()


def doc_image(path, img, fmt):
    from reportlab.lib.utils import ImageReader

    buf = BytesIO()
    img.save(buf, format=fmt)
    buf.seek(0)
    c = canvas.Canvas(path, pagesize=A4)
    c.drawImage(ImageReader(buf), 60, 400, width=400, height=300)
    page_text(c, f"An embedded {fmt} image")
    c.showPage()
    c.save()


def doc_shading(path):
    c = canvas.Canvas(path, pagesize=A4)
    c.linearGradient(60, 400, 500, 700, (
        __import__("reportlab.lib.colors", fromlist=["red"]).red,
        __import__("reportlab.lib.colors", fromlist=["blue"]).blue,
    ))
    page_text(c, "An axial shading")
    c.showPage()
    c.save()


# --------------------------------------------------------------- hand-built
#
# Everything reportlab will not write: a cross-reference stream carrying an
# object stream, the hybrid mixed history D-075 measured in mup.pdf, a
# standard font with no /Widths, an inline image, a tiling pattern, a mesh
# shading, and image codecs this renderer deliberately declines.


class Builder:
    """A minimal PDF writer whose offsets are computed, never typed."""

    def __init__(self):
        self.buf = bytearray(b"%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
        self.offsets = {}

    def obj(self, body, num=None):
        if num is None:
            num = len(self.offsets) + 1
        self.offsets[num] = len(self.buf)
        self.buf += b"%d 0 obj\n" % num
        self.buf += body if isinstance(body, bytes) else body.encode("latin-1")
        self.buf += b"\nendobj\n"
        return num

    def stream(self, dict_extra, data, num=None):
        body = b"<<" + dict_extra.encode("latin-1") + b"/Length %d>>\nstream\n" % len(data)
        body += data + b"\nendstream"
        return self.obj(body, num)

    def classic_xref(self, root, extra_trailer=""):
        start = len(self.buf)
        n = max(self.offsets) + 1
        self.buf += b"xref\n0 %d\n" % n
        self.buf += b"0000000000 65535 f \n"
        for i in range(1, n):
            self.buf += b"%010d 00000 n \n" % self.offsets.get(i, 0)
        self.buf += ("trailer\n<</Size %d/Root %d 0 R%s>>\nstartxref\n%d\n%%%%EOF\n"
                     % (n, root, extra_trailer, start)).encode("latin-1")
        return bytes(self.buf)


def simple_page_objects(b, content, page_extra="", resources="<</Font <</F1 3 0 R>>>>"):
    b.obj("<</Type /Catalog/Pages 2 0 R>>")            # 1
    b.obj("<</Type /Pages/Kids [4 0 R]/Count 1>>")     # 2
    b.obj("<</Type /Font/Subtype /Type1/BaseFont /Helvetica>>")  # 3
    content_num = 5
    b.obj("<</Type /Page/Parent 2 0 R/MediaBox [0 0 595 842]/Resources %s"
          "/Contents %d 0 R%s>>" % (resources, content_num, page_extra))  # 4
    b.stream("", content.encode("latin-1") if isinstance(content, str) else content)  # 5
    return b


def doc_font_no_widths(path):
    b = Builder()
    simple_page_objects(b, "BT /F1 18 Tf 60 700 Td (No Widths array anywhere) Tj ET\n"
                           "BT /F1 18 Tf 60 660 Td (so every advance is a guess) Tj ET")
    open(path, "wb").write(b.classic_xref(1))


def doc_inline_image(path):
    content = ("q 200 0 0 120 60 600 cm\n"
               "BI /W 4 /H 3 /CS /RGB /BPC 8 /F /AHx ID\n"
               "ff0000 00ff00 0000ff ffffff\n"
               "ff0000 00ff00 0000ff ffffff\n"
               "ff0000 00ff00 0000ff ffffff>\nEI Q\n"
               "BT /F1 12 Tf 60 560 Td (An inline image above) Tj ET")
    b = Builder()
    simple_page_objects(b, content)
    open(path, "wb").write(b.classic_xref(1))


def doc_tiling_pattern(path):
    b = Builder()
    b.obj("<</Type /Catalog/Pages 2 0 R>>")
    b.obj("<</Type /Pages/Kids [4 0 R]/Count 1>>")
    b.obj("<</Type /Font/Subtype /Type1/BaseFont /Helvetica>>")
    b.obj("<</Type /Page/Parent 2 0 R/MediaBox [0 0 595 842]"
          "/Resources <</Font <</F1 3 0 R>>/Pattern <</P1 6 0 R>>>>/Contents 5 0 R>>")
    b.stream("", b"/Pattern cs /P1 scn 60 400 400 300 re f\n"
                 b"BT /F1 12 Tf 60 360 Td (A tiling pattern above) Tj ET")
    cell = b"0 0 1 rg 0 0 5 5 re f 1 0 0 rg 5 5 5 5 re f"
    b.stream("/Type /Pattern/PatternType 1/PaintType 1/TilingType 1"
             "/BBox [0 0 10 10]/XStep 10/YStep 10/Resources <<>>", cell)
    open(path, "wb").write(b.classic_xref(1))


def doc_mesh_shading(path):
    # A type 4 (free-form Gouraud triangle) shading: one triangle whose
    # three corners are red, green and blue.
    def vertex(flag, x, y, r, g, bl):
        return struct.pack(">BHHBBB", flag, x, y, r, g, bl)

    data = vertex(0, 0, 0, 255, 0, 0) + vertex(0, 4000, 0, 0, 255, 0) + vertex(0, 2000, 4000, 0, 0, 255)
    b = Builder()
    b.obj("<</Type /Catalog/Pages 2 0 R>>")
    b.obj("<</Type /Pages/Kids [4 0 R]/Count 1>>")
    b.obj("<</Type /Font/Subtype /Type1/BaseFont /Helvetica>>")
    b.obj("<</Type /Page/Parent 2 0 R/MediaBox [0 0 595 842]"
          "/Resources <</Font <</F1 3 0 R>>/Shading <</S1 6 0 R>>>>/Contents 5 0 R>>")
    b.stream("", b"q 1 0 0 1 60 400 cm /S1 sh Q\n"
                 b"BT /F1 12 Tf 60 360 Td (A type 4 mesh shading above) Tj ET")
    b.stream("/ShadingType 4/ColorSpace /DeviceRGB/BitsPerCoordinate 16"
             "/BitsPerComponent 8/BitsPerFlag 8"
             "/Decode [0 500 0 300 0 1 0 1 0 1]", data)
    open(path, "wb").write(b.classic_xref(1))


def doc_undecodable_image(path, codec):
    """An image whose codec this renderer declines: the page must still draw."""
    b = Builder()
    b.obj("<</Type /Catalog/Pages 2 0 R>>")
    b.obj("<</Type /Pages/Kids [4 0 R]/Count 1>>")
    b.obj("<</Type /Font/Subtype /Type1/BaseFont /Helvetica>>")
    b.obj("<</Type /Page/Parent 2 0 R/MediaBox [0 0 595 842]"
          "/Resources <</Font <</F1 3 0 R>>/XObject <</Im1 6 0 R>>>>/Contents 5 0 R>>")
    b.stream("", b"q 300 0 0 200 60 500 cm /Im1 Do Q\n"
                 b"BT /F1 12 Tf 60 460 Td (An image this renderer declines) Tj ET\n"
                 b"60 440 400 2 re f")
    b.stream("/Type /XObject/Subtype /Image/Width 64/Height 64"
             "/ColorSpace /DeviceGray/BitsPerComponent 1/Filter /%s" % codec,
             b"\x00" * 512)
    open(path, "wb").write(b.classic_xref(1))


def doc_ccitt(path, k):
    """A CCITT-coded scan, encoded by Pillow rather than by anything here."""
    img = Image.new("1", (137, 71), 1)
    d = ImageDraw.Draw(img)
    d.rectangle([5, 5, 60, 30], fill=0)
    d.line([0, 40, 136, 55], fill=0)
    d.ellipse([70, 5, 130, 60], outline=0)
    for x in range(0, 137, 7):
        img.putpixel((x, 68), 0)
    buf = BytesIO()
    # Pillow writes group3/group4 TIFF with the opposite polarity from the
    # fax convention, so invert before encoding.
    inverted = img.point(lambda v: 0 if v else 1, mode="1")
    inverted.save(buf, format="TIFF", compression="group4" if k < 0 else "group3")
    tif = buf.getvalue()
    data = extract_tiff_strip(tif)

    b = Builder()
    b.obj("<</Type /Catalog/Pages 2 0 R>>")
    b.obj("<</Type /Pages/Kids [4 0 R]/Count 1>>")
    b.obj("<</Type /Font/Subtype /Type1/BaseFont /Helvetica>>")
    b.obj("<</Type /Page/Parent 2 0 R/MediaBox [0 0 595 842]"
          "/Resources <</Font <</F1 3 0 R>>/XObject <</Im1 6 0 R>>>>/Contents 5 0 R>>")
    b.stream("", b"q 411 0 0 213 60 500 cm /Im1 Do Q\n"
                 b"BT /F1 12 Tf 60 460 Td (A CCITT scan above) Tj ET")
    b.stream("/Type /XObject/Subtype /Image/Width 137/Height 71"
             "/ColorSpace /DeviceGray/BitsPerComponent 1/Filter /CCITTFaxDecode"
             "/DecodeParms <</K %d/Columns 137/Rows 71/BlackIs1 false>>" % k, data)
    open(path, "wb").write(b.classic_xref(1))


def extract_tiff_strip(tif):
    """Pull the single compressed strip out of a one-strip TIFF."""
    endian = "<" if tif[:2] == b"II" else ">"
    (ifd_off,) = struct.unpack(endian + "I", tif[4:8])
    (count,) = struct.unpack(endian + "H", tif[ifd_off:ifd_off + 2])
    offsets = counts = None
    for i in range(count):
        e = ifd_off + 2 + i * 12
        tag, typ, n = struct.unpack(endian + "HHI", tif[e:e + 8])
        (val,) = struct.unpack(endian + "I", tif[e + 8:e + 12])
        if typ == 3:
            (val,) = struct.unpack(endian + "H", tif[e + 8:e + 10])
        if tag == 273:
            offsets = val
        elif tag == 279:
            counts = val
    return tif[offsets:offsets + counts]


def doc_xref_stream(path):
    """A cross-reference stream carrying an object stream."""
    b = Builder()
    # Objects 1 (catalog) and 2 (pages node) live inside object stream 6.
    compressed = b"<</Type /Catalog/Pages 2 0 R>> <</Type /Pages/Kids [4 0 R]/Count 1>>"
    first = len(b"1 0 2 30 ")
    hdr = b"1 0 2 30 "
    # Recompute the second offset honestly.
    o1 = b"<</Type /Catalog/Pages 2 0 R>>"
    o2 = b"<</Type /Pages/Kids [4 0 R]/Count 1>>"
    hdr = b"1 0 2 %d " % (len(o1) + 1)
    first = len(hdr)
    objstm_data = hdr + o1 + b" " + o2

    b.obj("<</Type /Font/Subtype /Type1/BaseFont /Helvetica>>", num=3)
    b.obj("<</Type /Page/Parent 2 0 R/MediaBox [0 0 595 842]"
          "/Resources <</Font <</F1 3 0 R>>>>/Contents 5 0 R>>", num=4)
    b.stream("", b"BT /F1 14 Tf 60 700 Td (Objects 1 and 2 live in an object stream) Tj ET", num=5)
    b.stream("/Type /ObjStm/N 2/First %d/Filter /FlateDecode" % first,
             zlib.compress(objstm_data), num=6)

    xref_off = len(b.buf)
    entries = []
    entries.append(bytes([0, 0, 0, 0, 0xFF, 0xFF]))          # 0: free
    entries.append(bytes([2]) + struct.pack(">I", 6) + bytes([0]))  # 1: in objstm 6, index 0
    entries.append(bytes([2]) + struct.pack(">I", 6) + bytes([1]))  # 2: in objstm 6, index 1
    for n in (3, 4, 5, 6):
        entries.append(bytes([1]) + struct.pack(">I", b.offsets[n]) + bytes([0]))
    entries.append(bytes([1]) + struct.pack(">I", xref_off) + bytes([0]))  # 7: the xref stream
    data = zlib.compress(b"".join(entries))
    b.stream("/Type /XRef/Size 8/W [1 4 1]/Root 1 0 R/Filter /FlateDecode", data, num=7)
    b.buf += b"startxref\n%d\n%%%%EOF\n" % xref_off
    open(path, "wb").write(bytes(b.buf))


def doc_mixed_history(path):
    """D-075's shape: classic base, a hybrid /XRefStm revision, classic on top."""
    b = Builder()
    simple_page_objects(b, "BT /F1 14 Tf 60 700 Td (Revision 1) Tj ET")
    base = b.classic_xref(1)
    out = bytearray(base)

    # Revision 2: a classic "xref 0 0" stub pointing at a real xref stream.
    rev2_start = len(out)
    body_off = len(out)
    out += b"5 0 obj\n<</Length 52>>\nstream\nBT /F1 14 Tf 60 660 Td (Revision 2) Tj ET\nendstream\nendobj\n"
    xrefstm_off = len(out)
    entries = bytes([1]) + struct.pack(">I", body_off) + bytes([0])
    data = zlib.compress(entries)
    out += (b"8 0 obj\n<</Type /XRef/Size 9/Index [5 1]/W [1 4 1]/Root 1 0 R"
            b"/Prev %d/Filter /FlateDecode/Length %d>>\nstream\n" % (base.rfind(b"xref"), len(data)))
    out += data + b"\nendstream\nendobj\n"
    stub_off = len(out)
    out += (b"xref\n0 0\ntrailer\n<</Size 9/Root 1 0 R/Prev %d/XRefStm %d>>\nstartxref\n%d\n%%%%EOF\n"
            % (base.rfind(b"startxref") and int(base.split(b"startxref")[-1].split()[0]) or 0,
               xrefstm_off, stub_off))

    # Revision 3: plain classic on top.
    body3 = len(out)
    out += b"9 0 obj\n<</Comment (revision 3)>>\nendobj\n"
    x3 = len(out)
    out += (b"xref\n0 1\n0000000000 65535 f \n9 1\n%010d 00000 n \n"
            b"trailer\n<</Size 10/Root 1 0 R/Prev %d>>\nstartxref\n%d\n%%%%EOF\n"
            % (body3, stub_off, x3))
    open(path, "wb").write(bytes(out))


def doc_inherited_boxes(path):
    """MediaBox, Rotate and CropBox inherited from the Pages node."""
    b = Builder()
    b.obj("<</Type /Catalog/Pages 2 0 R>>")
    b.obj("<</Type /Pages/Kids [4 0 R]/Count 1/MediaBox [0 0 595 842]"
          "/CropBox [20 20 575 822]/Rotate 90>>")
    b.obj("<</Type /Font/Subtype /Type1/BaseFont /Helvetica>>")
    b.obj("<</Type /Page/Parent 2 0 R/Resources <</Font <</F1 3 0 R>>>>/Contents 5 0 R>>")
    b.stream("", b"BT /F1 14 Tf 60 700 Td (Boxes inherited from the Pages node) Tj ET\n"
                 b"0 0 1 rg 30 30 100 100 re f")
    open(path, "wb").write(b.classic_xref(1))


# ------------------------------------------------------------------- driver


def main():
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    out = sys.argv[1]
    os.makedirs(out, exist_ok=True)
    p = lambda name: os.path.join(out, name)

    # Page counts.
    for n in (1, 2, 5, 17, 50, 200, 500):
        doc_pages(p("pages-%03d.pdf" % n), n)

    # Page sizes.
    for name, size in (("a4", A4), ("a5", A5), ("a3", A3), ("letter", LETTER),
                       ("narrow", (200, 900)), ("tiny", (120, 120)), ("huge", (2000, 3000))):
        doc_pages(p("size-%s.pdf" % name), 1, size=size)

    # Rotations.
    for r in (0, 90, 180, 270):
        doc_pages(p("rotate-%03d.pdf" % r), 1, rotate=r)
    doc_pages(p("rotate-mixed.pdf"), 4, rotate=[0, 90, 180, 270])

    # Producers.
    for slug, producer in (("msprint", "Microsoft: Print To PDF"),
                           ("libreoffice", "LibreOffice 7.6"),
                           ("xerox", "Xerox WorkCentre Scan"),
                           ("sap", "SAP NetWeaver"),
                           ("euprava", "eUprava Signing Applet")):
        doc_producer(p("producer-%s.pdf" % slug), producer)

    # Text and fonts.
    doc_pages(p("text-cyrillic-std14.pdf"), 1, text="Cyrillic below")
    doc_pages(p("text-latin-std14.pdf"), 1, text=LATIN)
    doc_pages(p("text-times-std14.pdf"), 1, text=LATIN, font="Times-Roman")
    ttf = find_embeddable_font()
    if ttf:
        doc_embedded_font(p("text-embedded-truetype.pdf"), ttf)
    else:
        print("no TrueType font found to embed; skipping text-embedded-truetype.pdf")
    doc_font_no_widths(p("font-no-widths.pdf"))

    # Cross-reference mechanisms.
    doc_xref_stream(p("xref-stream-objstm.pdf"))
    doc_mixed_history(p("xref-mixed-history.pdf"))

    # Images.
    rgb = Image.new("RGB", (200, 150))
    d = ImageDraw.Draw(rgb)
    for x in range(200):
        d.line([(x, 0), (x, 150)], fill=(x, 255 - x, 128))
    doc_image(p("img-jpeg.pdf"), rgb, "JPEG")
    rgba = rgb.convert("RGBA")
    rgba.putalpha(Image.linear_gradient("L").resize((200, 150)))
    doc_image(p("img-flate-smask.pdf"), rgba, "PNG")
    doc_ccitt(p("img-ccitt-g4.pdf"), -1)
    doc_ccitt(p("img-ccitt-g3.pdf"), 0)
    doc_undecodable_image(p("img-jbig2.pdf"), "JBIG2Decode")
    doc_undecodable_image(p("img-jpx.pdf"), "JPXDecode")

    # Graphics this renderer approximates.
    doc_shading(p("gfx-axial-shading.pdf"))
    doc_tiling_pattern(p("gfx-shading-pattern-inline.pdf"))
    doc_mesh_shading(p("gfx-mesh-shading.pdf"))
    doc_inline_image(p("gfx-inline-image.pdf"))

    # Boxes.
    doc_inherited_boxes(p("box-inherited.pdf"))

    files = sorted(f for f in os.listdir(out) if f.endswith(".pdf"))
    print("%d documents in %s" % (len(files), out))
    for f in files:
        print("  %-32s %8d" % (f, os.path.getsize(os.path.join(out, f))))
    return 0


if __name__ == "__main__":
    sys.exit(main())
