import sys
from playwright.sync_api import sync_playwright
with sync_playwright() as p:
    b = p.chromium.launch(); pg = b.new_page(viewport={'width': 1200, 'height': 800}, device_scale_factor=2)
    pg.goto('file:///tmp/guide/guide.html'); pg.wait_for_timeout(800)
    pg.screenshot(path=sys.argv[1], full_page=True); b.close()
