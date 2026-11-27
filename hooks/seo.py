"""MkDocs hooks that date the sitemap entries, fix the homepage's social titles and clean up
the head Material renders."""

import functools
import html
import logging
import os
import re
import subprocess

from mkdocs.plugins import event_priority

# Material adds a relative alternate link for each extra.alternate entry, identical on every
# page, so it does not describe the page. This pattern matches Material's attribute order:
# rel, href, hreflang. The links in overrides/main.html use rel, hreflang, href and stay.
MATERIAL_HREFLANG = re.compile(
    r'\s*<link rel="alternate" href="[^"]*" hreflang="[^"]*">'
)
# Any link tag that starts with rel="alternate" and has an hreflang attribute.
HEAD_ALTERNATE = re.compile(r'<link rel="alternate"[^>]*hreflang="[^"]*"[^>]*>')
TITLE_TAG = re.compile(r'(<meta property="(?:og|twitter):title" content=")([^"]*)(")')
# The social plugin writes its twitter meta tags with a property attribute only. X documents
# these tags with a name attribute, so on_post_page adds one.
TWITTER_META = re.compile(r'<meta property="(twitter:[a-z:]+)"')
# A warning on this logger fails `mkdocs build --strict`.
log = logging.getLogger("mkdocs.hooks.seo")
SEARCH_LABEL = {"pl": "Szukaj", "en": "Search"}


def _git(root, *args):
    out = subprocess.run(["git", *args], cwd=root, capture_output=True, text=True)
    return out.stdout.strip() if out.returncode == 0 else ""


@functools.lru_cache(maxsize=None)
def _lastmod(root, path):
    """Return the date (YYYY-MM-DD) of the last commit touching `path`, or "" if unknown.

    A shallow checkout counts as unknown, because its cut-short history gives wrong dates.
    """
    if _git(root, "rev-parse", "--is-shallow-repository") != "false":
        return ""
    return _git(root, "log", "-1", "--format=%cs", "--", path)


def on_page_markdown(markdown, page, config, files):
    # MkDocs renders sitemap.xml before it calls on_page_context for any page, so lastmod
    # is set in this event.
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
    # After the removal, only the template's own links, with absolute https hrefs, may remain.
    stray = [a for a in HEAD_ALTERNATE.findall(head) if 'href="https://' not in a]
    # A dialog tag that still ends after its role attribute means the label replacement
    # above did not match.
    if stray or 'role="dialog">' in output:
        log.warning(f"{where}: Material markup changed, update hooks/seo.py patterns")
    return output


# The social plugin adds its meta tags in its own on_post_page, which runs earlier.
@event_priority(-100)
def on_post_page(output, page, config):
    output = _clean(output, config, page.file.src_path)
    if page.is_homepage and page.meta.get("title"):
        # On the homepage the social plugin uses the bare site name as the title. This sets
        # og:title and twitter:title to the page title plus site name, as in the title tag.
        full = html.escape(f"{page.meta['title']} - {config.site_name}")
        output = TITLE_TAG.sub(lambda m: f"{m.group(1)}{full}{m.group(3)}", output)
    return TWITTER_META.sub(r'<meta name="\1" property="\1"', output)


def on_post_template(output_content, template_name, config):
    # 404.html is a theme template, rendered without on_post_page
    if template_name == "404.html":
        return _clean(output_content, config, template_name)
    return output_content
