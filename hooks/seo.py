"""MkDocs hooks for the bilingual site (PL at /, EN at /en/).

- Sitemap lastmod is the date of the last commit touching the page's source.
- The homepage og:title / twitter:title are made to match <title>.
- Post-render cleanup: drop Material's relative hreflang links (hreflang pairs
  live in overrides/sitemap.xml) and name the search dialog for screen readers.
  404.html gets the same cleanup via on_post_template.
"""

import functools
import html
import logging
import os
import re
import subprocess

from mkdocs.plugins import event_priority

# Material emits <link rel="alternate" href="/" hreflang="pl"> for extra.alternate:
# relative, and the same on every page, so wrong for hreflang.
MATERIAL_HREFLANG = re.compile(
    r'\s*<link rel="alternate" href="[^"]*" hreflang="[^"]*">'
)
TITLE_TAG = re.compile(r'(<meta property="(?:og|twitter):title" content=")([^"]*)(")')
# the social plugin emits twitter:* with property= only; crawlers read name=
TWITTER_META = re.compile(r'<meta property="(twitter:[a-z:]+)"')
log = logging.getLogger("mkdocs.hooks.seo")  # warnings fail `mkdocs build --strict`
SEARCH_LABEL = {"pl": "Szukaj", "en": "Search"}


def _git(root, *args):
    out = subprocess.run(["git", *args], cwd=root, capture_output=True, text=True)
    return out.stdout.strip() if out.returncode == 0 else ""


@functools.lru_cache(maxsize=None)
def _lastmod(root, path):
    """Date of the last commit touching `path`.

    Empty on a shallow checkout, where every file would get the newest commit date.
    """
    if _git(root, "rev-parse", "--is-shallow-repository") != "false":
        return ""
    return _git(root, "log", "-1", "--format=%cs", "--", path)


def on_page_markdown(markdown, page, config, files):
    # sitemap.xml is rendered before on_page_context, so lastmod is set here
    repo_root = os.path.dirname(os.path.abspath(config.config_file_path))
    rel = os.path.relpath(os.path.join(config.docs_dir, page.file.src_path), repo_root)
    page.meta["lastmod"] = _lastmod(repo_root, rel)
    return markdown


def _clean(output, config, where):
    output = MATERIAL_HREFLANG.sub("", output)
    label = SEARCH_LABEL.get(config.theme["language"], "Search")
    output = output.replace(
        'data-md-component="search" role="dialog"',
        f'data-md-component="search" role="dialog" aria-label="{label}"',
    )
    head = output.split("</head>", 1)[0]
    if "hreflang=" in head or 'role="dialog">' in output:  # Material markup changed
        log.warning(f"{where}: Material markup changed, update hooks/seo.py patterns")
    return output


@event_priority(-100)  # after the social plugin has injected its meta tags
def on_post_page(output, page, config):
    output = _clean(output, config, page.file.src_path)
    if page.is_homepage and page.meta.get("title"):
        # the social plugin uses the bare site name on the homepage, while <title> is
        # "<page title> - <site name>": make og:title / twitter:title match <title>
        full = html.escape(f"{page.meta['title']} - {config.site_name}")
        output = TITLE_TAG.sub(lambda m: f"{m.group(1)}{full}{m.group(3)}", output)
    return TWITTER_META.sub(r'<meta name="\1" property="\1"', output)


def on_post_template(output_content, template_name, config):
    # 404.html is a theme template, rendered without on_post_page
    if template_name == "404.html":
        return _clean(output_content, config, template_name)
    return output_content
