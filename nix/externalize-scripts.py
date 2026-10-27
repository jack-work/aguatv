import hashlib
import pathlib
import re
import sys

INLINE = re.compile(r"<script(?P<attrs>(?![^>]*\bsrc=)[^>]*)>(?P<body>.*?)</script>", re.S | re.I)
DATA = re.compile(r"\btype\s*=\s*[\"']application/json[\"']", re.I)

root = pathlib.Path(sys.argv[1])
for page in sorted(root.glob("*.html")):
    text = page.read_text(encoding="utf-8")
    moved = []

    def externalize(m):
        if DATA.search(m.group("attrs")):
            return m.group(0)
        body = m.group("body")
        name = "inline-" + hashlib.sha256(body.encode("utf-8")).hexdigest()[:12] + ".js"
        (root / name).write_text(body, encoding="utf-8")
        moved.append(name)
        return f'<script src="{name}"{m.group("attrs")}></script>'

    text = INLINE.sub(externalize, text)
    if INLINE.search(text) and any(not DATA.search(m.group("attrs")) for m in INLINE.finditer(text)):
        sys.exit(f"{page.name}: an inline script survived")
    page.write_text(text, encoding="utf-8")
    print(page.name, len(moved), "inline scripts moved to files")
