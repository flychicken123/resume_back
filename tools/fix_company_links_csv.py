import csv
import re
import sys
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass
from functools import lru_cache
from typing import Dict, Iterable, List, Optional, Tuple


USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/118.0.0.0 Safari/537.36"
)

FETCH_TIMEOUT_S = 25
VALIDATION_TIMEOUT_S = 6
MAX_HTML_BYTES = 2_000_000

RE_LEVER_URL = re.compile(r"https?://jobs\.lever\.co/([a-z0-9-]+)(?:/|[\"'\s<>]|$)", re.I)
RE_LEVER_API = re.compile(r"https?://api\.lever\.co/v0/postings/([a-z0-9-]+)(?:\?|/|[\"'\s<>]|$)", re.I)
RE_LEVER_ACCOUNT = re.compile(r"leverJobsOptions\s*=\s*\{[^}]*accountName\s*:\s*[\"']([^\"']+)[\"']", re.I)

RE_GH_BOARD = re.compile(r"https?://boards\.greenhouse\.io/([a-z0-9_-]+)(?:/|[\"'\s<>]|$)", re.I)
RE_GH_JOB_BOARD = re.compile(r"https?://job-boards\.greenhouse\.io/([a-z0-9_-]+)(?:/|[\"'\s<>]|$)", re.I)
RE_GH_API = re.compile(r"boards-api\.greenhouse\.io/v1/boards/([a-z0-9_-]+)", re.I)

RE_ASHBY = re.compile(r"https?://jobs\.ashbyhq\.com/([a-z0-9_-]+)(?:/|[\"'\s<>]|$)", re.I)
RE_WORKABLE = re.compile(r"https?://apply\.workable\.com/([a-z0-9_-]+)(?:/|[\"'\s<>]|$)", re.I)
RE_SMARTRECRUITERS = re.compile(r"https?://(?:www\.)?smartrecruiters\.com/([a-z0-9_-]+)(?:/|[\"'\s<>]|$)", re.I)

RE_WORKDAY_URL = re.compile(r"https?://[a-z0-9-]+\.wd\d+\.myworkdayjobs\.com/[^\n\"'<>\s]+", re.I)


@dataclass(frozen=True)
class DetectedATS:
    provider: str
    careers_url: str


def slugify(value: str) -> str:
    value = (value or "").strip().lower()
    value = re.sub(r"[^a-z0-9]+", "-", value)
    value = value.strip("-")
    return value


def normalize_url(value: str) -> str:
    value = (value or "").strip()
    if not value:
        return ""
    if "://" not in value:
        value = "https://" + value
    return value


def url_domain(value: str) -> str:
    value = normalize_url(value)
    try:
        parsed = urllib.parse.urlparse(value)
    except Exception:
        return ""
    return (parsed.netloc or "").lower()


def is_provider_url(provider: str, careers_url: str) -> bool:
    provider = (provider or "").strip().lower()
    host = url_domain(careers_url)
    if not host:
        return False
    if provider == "greenhouse":
        return "boards.greenhouse.io" in host or "job-boards.greenhouse.io" in host
    if provider == "lever":
        return "jobs.lever.co" in host
    if provider == "workday":
        return "myworkdayjobs.com" in host or "workday" in host
    if provider == "smartrecruiters":
        return "smartrecruiters.com" in host
    if provider == "ashby":
        return "ashbyhq.com" in host
    if provider == "workable":
        return "workable.com" in host
    if provider == "bamboohr":
        return "bamboohr.com" in host
    return False


def provider_from_careers_url(careers_url: str) -> str:
    host = url_domain(careers_url)
    if not host:
        return ""
    if "boards.greenhouse.io" in host or "job-boards.greenhouse.io" in host:
        return "greenhouse"
    if "jobs.lever.co" in host:
        return "lever"
    if "myworkdayjobs.com" in host or "workday" in host:
        return "workday"
    if "smartrecruiters.com" in host:
        return "smartrecruiters"
    if "ashbyhq.com" in host:
        return "ashby"
    if "workable.com" in host:
        return "workable"
    if "bamboohr.com" in host:
        return "bamboohr"
    return ""


def canonicalize_careers_url(provider: str, careers_url: str) -> str:
    provider = (provider or "").strip().lower()
    careers_url = normalize_url(careers_url)
    if not careers_url:
        return ""
    parsed = urllib.parse.urlparse(careers_url)
    host = (parsed.netloc or "").lower()
    if provider == "greenhouse" and "job-boards.greenhouse.io" in host:
        parsed = parsed._replace(netloc=host.replace("job-boards.greenhouse.io", "boards.greenhouse.io"))
        return parsed.geturl()
    if provider == "workday":
        segments = [s for s in (parsed.path or "").split("/") if s]
        if segments:
            for i, seg in enumerate(segments):
                if seg.lower() in ("job", "jobs"):
                    if i > 0:
                        segments = segments[:i]
                    break
            if len(segments) >= 2 and re.match(r"^[a-z]{2}-[A-Z]{2}$", segments[0]):
                segments = segments[:2]
            parsed = parsed._replace(path="/" + "/".join(segments))
            return parsed.geturl().rstrip("/")
    return careers_url


def _first_path_segment(url: str) -> str:
    url = normalize_url(url)
    try:
        parsed = urllib.parse.urlparse(url)
    except Exception:
        return ""
    segments = [s for s in (parsed.path or "").split("/") if s]
    return segments[0].strip() if segments else ""


def _token_from_host(host: str) -> str:
    host = (host or "").strip().lower()
    if not host:
        return ""
    host = host.split(":")[0]
    parts = [p for p in host.split(".") if p]
    if not parts:
        return ""
    if parts[0] in ("www", "jobs", "careers") and len(parts) >= 2:
        return parts[1]
    return parts[0]


def _is_plausible_slug(value: str) -> bool:
    value = (value or "").strip().lower()
    if not value:
        return False
    if value in ("www", "com", "io", "net", "org", "jobs", "careers"):
        return False
    if len(value) < 2:
        return False
    return True


@lru_cache(maxsize=2048)
def greenhouse_board_exists(token: str) -> Optional[bool]:
    token = (token or "").strip()
    if not token:
        return None
    endpoint = f"https://boards-api.greenhouse.io/v1/boards/{urllib.parse.quote(token)}/jobs?per_page=1"
    req = urllib.request.Request(endpoint, headers={"Accept": "application/json", "User-Agent": USER_AGENT}, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=VALIDATION_TIMEOUT_S) as resp:
            if resp.status != 200:
                return None
            payload = resp.read(MAX_HTML_BYTES).decode("utf-8", errors="replace")
            data = __import__("json").loads(payload)
            return isinstance(data, dict) and "jobs" in data
    except urllib.error.HTTPError as e:
        if getattr(e, "code", None) == 404:
            return False
        return None
    except Exception:
        return None


@lru_cache(maxsize=2048)
def lever_account_exists(slug: str) -> Optional[bool]:
    slug = (slug or "").strip()
    if not slug:
        return None
    endpoint = f"https://api.lever.co/v0/postings/{urllib.parse.quote(slug)}?mode=json&limit=1"
    req = urllib.request.Request(endpoint, headers={"Accept": "application/json", "User-Agent": USER_AGENT}, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=VALIDATION_TIMEOUT_S) as resp:
            if resp.status != 200:
                return None
            payload = resp.read(MAX_HTML_BYTES).decode("utf-8", errors="replace")
            data = __import__("json").loads(payload)
            return isinstance(data, list)
    except urllib.error.HTTPError as e:
        if getattr(e, "code", None) == 404:
            return False
        return None
    except Exception:
        return None


@lru_cache(maxsize=2048)
def ashby_company_exists(slug: str) -> Optional[bool]:
    slug = (slug or "").strip()
    if not slug:
        return None
    url = f"https://jobs.ashbyhq.com/{urllib.parse.quote(slug)}"
    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/html,*/*",
            "Accept-Language": "en-US,en;q=0.9",
        },
        method="GET",
    )
    try:
        with urllib.request.urlopen(req, timeout=VALIDATION_TIMEOUT_S) as resp:
            if resp.status != 200:
                return None
            html = resp.read(50_000).decode("utf-8", errors="replace").lower()
            if "cdn.ashbyprd.com" in html or "ashby" in html:
                return True
            if "not found" in html:
                return False
            return True
    except urllib.error.HTTPError as e:
        if getattr(e, "code", None) == 404:
            return False
        return None
    except Exception:
        return None


@lru_cache(maxsize=4096)
def url_seems_alive(url: str) -> bool:
    url = normalize_url(url)
    if not url:
        return False
    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/html,*/*",
            "Accept-Language": "en-US,en;q=0.9",
        },
        method="GET",
    )
    try:
        with urllib.request.urlopen(req, timeout=VALIDATION_TIMEOUT_S) as resp:
            _ = resp.read(1024)
            return 200 <= resp.status < 400
    except Exception:
        return False


@lru_cache(maxsize=4096)
def workday_url_seems_valid(url: str) -> Optional[bool]:
    url = canonicalize_careers_url("workday", url)
    url = normalize_url(url)
    if not url:
        return None
    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/html,*/*",
            "Accept-Language": "en-US,en;q=0.9",
        },
        method="GET",
    )
    try:
        with urllib.request.urlopen(req, timeout=VALIDATION_TIMEOUT_S) as resp:
            _ = resp.read(1024)
            return True
    except urllib.error.HTTPError as e:
        if getattr(e, "code", None) == 404:
            return False
        return None
    except urllib.error.URLError:
        return False
    except Exception:
        return None


def provider_url_valid(provider: str, careers_url: str) -> Optional[bool]:
    provider = (provider or "").strip().lower()
    careers_url = (careers_url or "").strip()
    if not provider or not careers_url:
        return None

    if provider == "greenhouse":
        token = _first_path_segment(careers_url)
        return greenhouse_board_exists(token) if token else None
    if provider == "lever":
        slug = _first_path_segment(careers_url)
        return lever_account_exists(slug) if slug else None
    if provider == "ashby":
        slug = _first_path_segment(careers_url)
        return ashby_company_exists(slug) if slug else None
    if provider == "workday":
        return workday_url_seems_valid(careers_url)

    return None


def guess_ats_from_name_and_domain(name: str, careers_url: str, website_url: str) -> Optional[DetectedATS]:
    candidates: List[str] = []
    for raw in (url_domain(careers_url), url_domain(website_url)):
        if raw:
            base = _token_from_host(raw)
            if _is_plausible_slug(base) and base not in candidates:
                candidates.append(base)
    s = slugify(name)
    if s and s not in candidates:
        candidates.append(s)
    # Also try a compact version for names with hyphens.
    if s and "-" in s:
        compact = s.replace("-", "")
        if compact and compact not in candidates:
            candidates.append(compact)

    for token in candidates:
        if greenhouse_board_exists(token) is True:
            return DetectedATS("greenhouse", f"https://boards.greenhouse.io/{token}")
    for slug in candidates:
        if lever_account_exists(slug) is True:
            return DetectedATS("lever", f"https://jobs.lever.co/{slug}")
    for slug in candidates:
        if ashby_company_exists(slug) is True:
            return DetectedATS("ashby", f"https://jobs.ashbyhq.com/{slug}")
    return None


def fetch_html(url: str) -> Tuple[str, str]:
    def request_once(target: str) -> Tuple[str, str]:
        req = urllib.request.Request(
            target,
            headers={
                "User-Agent": USER_AGENT,
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
                "Accept-Language": "en-US,en;q=0.9",
            },
            method="GET",
        )
        with urllib.request.urlopen(req, timeout=FETCH_TIMEOUT_S) as resp:
            final_url = resp.geturl()
            raw = resp.read(MAX_HTML_BYTES)
            encoding = resp.headers.get_content_charset() or "utf-8"
            return final_url, raw.decode(encoding, errors="replace")

    url = normalize_url(url)

    # Follow 308 manually (urllib may not handle it on older runtimes).
    target = url
    last_error: Optional[Exception] = None
    for attempt in range(3):
        try:
            for _ in range(10):
                try:
                    return request_once(target)
                except urllib.error.HTTPError as e:
                    code = getattr(e, "code", None)
                    if code in (301, 302, 303, 307, 308):
                        loc = e.headers.get("Location") if e.headers else None
                        if not loc:
                            break
                        target = urllib.parse.urljoin(target, loc)
                        continue
                    raw = e.read(MAX_HTML_BYTES)
                    return e.geturl(), raw.decode("utf-8", errors="replace")
        except Exception as e:
            last_error = e
            time.sleep(0.25 * (attempt + 1))

    if last_error:
        raise last_error
    raise RuntimeError("fetch failed")


def choose_best_workday_url(candidates: Iterable[str]) -> Optional[str]:
    unique: List[str] = []
    for c in candidates:
        c = c.strip()
        if not c:
            continue
        if c not in unique:
            unique.append(c)
    if not unique:
        return None
    unique.sort(key=lambda u: ("/wday/cxs/" in u, len(u)))
    # Prefer a site landing page (not the CXS API endpoint).
    for u in unique:
        if "/wday/cxs/" in u:
            continue
        return u
    return unique[0]


def detect_from_html(final_url: str, html: str) -> Optional[DetectedATS]:
    html = html or ""

    if match := RE_LEVER_URL.search(html):
        slug = match.group(1).strip()
        return DetectedATS("lever", f"https://jobs.lever.co/{slug}")
    if match := RE_LEVER_API.search(html):
        slug = match.group(1).strip()
        return DetectedATS("lever", f"https://jobs.lever.co/{slug}")
    if match := RE_LEVER_ACCOUNT.search(html):
        slug = match.group(1).strip()
        if slug:
            return DetectedATS("lever", f"https://jobs.lever.co/{slug}")

    if match := RE_GH_BOARD.search(html):
        token = match.group(1).strip()
        return DetectedATS("greenhouse", f"https://boards.greenhouse.io/{token}")
    if match := RE_GH_JOB_BOARD.search(html):
        token = match.group(1).strip()
        return DetectedATS("greenhouse", f"https://boards.greenhouse.io/{token}")
    if match := RE_GH_API.search(html):
        token = match.group(1).strip()
        return DetectedATS("greenhouse", f"https://boards.greenhouse.io/{token}")

    if match := RE_ASHBY.search(html):
        slug = match.group(1).strip()
        return DetectedATS("ashby", f"https://jobs.ashbyhq.com/{slug}")

    if match := RE_WORKABLE.search(html):
        slug = match.group(1).strip()
        return DetectedATS("workable", f"https://apply.workable.com/{slug}/")

    if match := RE_SMARTRECRUITERS.search(html):
        slug = match.group(1).strip()
        return DetectedATS("smartrecruiters", f"https://www.smartrecruiters.com/{slug}")

    workday_urls = RE_WORKDAY_URL.findall(html)
    best = choose_best_workday_url(workday_urls)
    if best:
        return DetectedATS("workday", best)

    # If the final URL redirected straight to a known ATS host, prefer it.
    final_host = url_domain(final_url)
    if "jobs.lever.co" in final_host:
        return DetectedATS("lever", final_url.split("?", 1)[0].rstrip("/"))
    if "boards.greenhouse.io" in final_host or "job-boards.greenhouse.io" in final_host:
        parts = urllib.parse.urlparse(final_url)
        token = parts.path.strip("/").split("/", 1)[0].strip()
        if token:
            return DetectedATS("greenhouse", f"https://boards.greenhouse.io/{token}")
    if "myworkdayjobs.com" in final_host:
        return DetectedATS("workday", canonicalize_careers_url("workday", final_url.split("?", 1)[0]))
    if "smartrecruiters.com" in final_host:
        return DetectedATS("smartrecruiters", final_url.split("?", 1)[0])
    if "ashbyhq.com" in final_host:
        return DetectedATS("ashby", final_url.split("?", 1)[0])
    if "workable.com" in final_host:
        return DetectedATS("workable", final_url.split("?", 1)[0])

    return None


def should_attempt_fix(row: Dict[str, str]) -> bool:
    provider = (row.get("ats_provider") or "").strip().lower()
    careers_url = (row.get("careers_url") or "").strip()

    if provider in ("", "auto", "custom"):
        return True
    if not is_provider_url(provider, careers_url):
        return True
    validity = provider_url_valid(provider, careers_url)
    return validity is False


def _candidate_discovery_urls(row: Dict[str, str], fallback_careers_url: str) -> List[str]:
    urls: List[str] = []
    careers_url = (row.get("careers_url") or "").strip()
    website_url = (row.get("website_url") or "").strip()

    for u in (careers_url, fallback_careers_url, website_url):
        u = (u or "").strip()
        if u and u not in urls:
            urls.append(u)

    # A few common paths for discovery.
    base = normalize_url(website_url).rstrip("/")
    if base:
        for suffix in ("/careers", "/careers/", "/jobs", "/jobs/", "/about/jobs", "/about/jobs/"):
            u = base + suffix
            if u not in urls:
                urls.append(u)

        host = url_domain(base).split(":")[0]
        host_parts = [p for p in host.split(".") if p]
        if len(host_parts) == 2 and not host_parts[0].endswith("group"):
            group_host = f"{host_parts[0]}group.{host_parts[1]}"
            for prefix in (f"https://{group_host}", f"https://www.{group_host}"):
                for suffix in ("/careers", "/careers/", "/jobs", "/jobs/"):
                    u = prefix + suffix
                    if u not in urls:
                        urls.append(u)

    return urls


def fix_rows(rows: List[Dict[str, str]], fallback_careers_by_name: Optional[Dict[str, str]] = None) -> Tuple[List[Dict[str, str]], List[str]]:
    changes: List[str] = []
    out: List[Dict[str, str]] = []

    for row in rows:
        row = dict(row)
        name = (row.get("name") or "").strip()
        careers_url = (row.get("careers_url") or "").strip()
        provider = (row.get("ats_provider") or "").strip().lower()
        website_url = (row.get("website_url") or "").strip()
        fallback_url = ""
        if fallback_careers_by_name is not None:
            fallback_url = (fallback_careers_by_name.get(name) or "").strip()

        if not should_attempt_fix(row):
            provider_norm = provider_from_careers_url(careers_url)
            row["careers_url"] = canonicalize_careers_url(provider_norm or provider, careers_url)
            row["domain"] = url_domain(row["careers_url"])
            row["provider_from_domain"] = provider_norm or provider
            out.append(row)
            continue

        if not careers_url:
            out.append(row)
            continue

        detected: Optional[DetectedATS] = None
        discovery_urls = _candidate_discovery_urls(row, fallback_url)
        for u in discovery_urls:
            try:
                final_url, html = fetch_html(u)
                candidate = detect_from_html(final_url, html)
                if candidate is None:
                    continue
                validity = provider_url_valid(candidate.provider, candidate.careers_url)
                if validity is False:
                    continue
                detected = candidate
                break
            except Exception:
                continue

        if detected is None:
            detected = guess_ats_from_name_and_domain(name, careers_url, website_url)

        if detected is None:
            provider_norm = provider_from_careers_url(careers_url)
            row["careers_url"] = canonicalize_careers_url(provider_norm or provider, careers_url)
            row["domain"] = url_domain(row["careers_url"])
            row["provider_from_domain"] = provider_norm or provider
            out.append(row)
            continue

        new_provider = detected.provider
        new_url = canonicalize_careers_url(new_provider, detected.careers_url)

        if provider != new_provider or careers_url != new_url:
            changes.append(f"{name}: {provider or '<none>'} {careers_url} -> {new_provider} {new_url}")
            row["ats_provider"] = new_provider
            row["careers_url"] = new_url
            row["domain"] = url_domain(new_url)
            row["provider_from_domain"] = new_provider
        else:
            row["careers_url"] = canonicalize_careers_url(new_provider, careers_url)
            row["domain"] = url_domain(row["careers_url"])
            row["provider_from_domain"] = new_provider

        out.append(row)

    return out, changes


def read_csv(path: str) -> Tuple[List[Dict[str, str]], List[str]]:
    with open(path, newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        rows = list(reader)
        return rows, list(reader.fieldnames or [])


def write_csv(path: str, rows: List[Dict[str, str]], fieldnames: List[str]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(rows)


def main(argv: List[str]) -> int:
    if len(argv) < 3:
        print("usage: fix_company_links_csv.py <input.csv> <output.csv> [--fallback-csv <fallback.csv>]", file=sys.stderr)
        return 2

    src = argv[1]
    dst = argv[2]

    fallback: Optional[Dict[str, str]] = None
    if "--fallback-csv" in argv[3:]:
        idx = argv.index("--fallback-csv")
        if idx + 1 >= len(argv):
            print("missing --fallback-csv value", file=sys.stderr)
            return 2
        fallback_path = argv[idx + 1]
        fb_rows, _ = read_csv(fallback_path)
        fallback = {}
        for r in fb_rows:
            n = (r.get("name") or "").strip()
            u = (r.get("careers_url") or "").strip()
            if n and u:
                fallback[n] = u

    rows, fieldnames = read_csv(src)
    if not fieldnames:
        print("empty CSV header", file=sys.stderr)
        return 2

    started = time.time()
    fixed, changes = fix_rows(rows, fallback_careers_by_name=fallback)
    write_csv(dst, fixed, fieldnames)
    elapsed = time.time() - started

    print(f"wrote: {dst}")
    print(f"rows: {len(rows)}; changed: {sum(1 for c in changes if '->' in c)}; elapsed_s: {elapsed:.2f}")
    if changes:
        print("--- changes (first 200) ---")
        for line in changes[:200]:
            print(line)

    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
