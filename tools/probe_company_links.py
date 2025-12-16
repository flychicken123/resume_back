import csv
import re
import sys
import urllib.request


USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/118.0.0.0 Safari/537.36"
)

MAX_BYTES = 2_000_000

RE_GH = re.compile(r"(?:https?:\\/\\/)?boards\\.greenhouse\\.io(?:\\/|/)([a-z0-9_-]+)", re.I)
RE_GH2 = re.compile(r"(?:https?:\\/\\/)?job-boards\\.greenhouse\\.io(?:\\/|/)([a-z0-9_-]+)", re.I)
RE_LEVER = re.compile(r"(?:https?:\\/\\/)?jobs\\.lever\\.co(?:\\/|/)([a-z0-9-]+)", re.I)
RE_WD = re.compile(r"(https?:\\/\\/[^\"\\s<>]*myworkdayjobs\\.com[^\"\\s<>]*)", re.I)


def fetch(url: str) -> tuple[str, str]:
    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/html,*/*",
            "Accept-Language": "en-US,en;q=0.9",
        },
        method="GET",
    )
    with urllib.request.urlopen(req, timeout=25) as resp:
        final_url = resp.geturl()
        raw = resp.read(MAX_BYTES)
        return final_url, raw.decode("utf-8", errors="replace")


def main(argv: list[str]) -> int:
    if len(argv) < 3:
        print("usage: probe_company_links.py <input.csv> <name1> [name2...]", file=sys.stderr)
        return 2
    path = argv[1]
    targets = set(argv[2:])
    with open(path, newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            name = (row.get("name") or "").strip()
            if name not in targets:
                continue
            url = (row.get("careers_url") or "").strip()
            print(f"\n== {name}\n{url}")
            try:
                final, html = fetch(url)
            except Exception as e:
                print(f"fetch_err: {e}")
                continue
            gh = sorted(set(RE_GH.findall(html)) | set(RE_GH2.findall(html)))
            lv = sorted(set(RE_LEVER.findall(html)))
            wd = RE_WD.findall(html)
            print(f"final: {final}")
            print(f"html_len: {len(html)}")
            print(f"greenhouse_tokens: {gh[:5]}")
            print(f"lever_slugs: {lv[:5]}")
            print(f"workday_urls: {wd[:2]}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))

