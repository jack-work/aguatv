import base64
import hashlib
import pathlib
import re
import sys

INLINE = re.compile(r"<script(?![^>]*\bsrc=)[^>]*>(.*?)</script>", re.S | re.I)

root = pathlib.Path(sys.argv[1])
for page in sorted(root.glob("*.html")):
    text = page.read_text(encoding="utf-8")
    hashes = sorted({
        "'sha256-" + base64.b64encode(hashlib.sha256(body.encode("utf-8")).digest()).decode() + "'"
        for body in INLINE.findall(text)
    })
    (root / (page.name + ".csp")).write_text(" ".join(hashes) + "\n", encoding="utf-8")
    print(page.name, len(hashes), "inline scripts")
