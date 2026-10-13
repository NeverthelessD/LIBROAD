import time, json
from playwright.sync_api import sync_playwright
boxes = {}
with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={'width': 1320, 'height': 860}, color_scheme='light', device_scale_factor=2)
    pg.goto("http://127.0.0.1:18990/?t=t"); time.sleep(4)
    pg.fill('#q', '보고서'); time.sleep(1.2)
    pg.wait_for_timeout(500)
    pg.click('#resList .row >> nth=0'); time.sleep(0.4)
    pg.click('#resList .row >> nth=1', modifiers=['Meta']); time.sleep(1.2)
    pg.evaluate("""fetch('/api/convert/run',{method:'POST',headers:{'x-token':'t'},body:JSON.stringify({paths:['/tmp/cv/home/Documents/회사/보도자료_누리호.hwp','/tmp/cv/home/Documents/개인/리포트 양식.hwp','/tmp/cv/home/Documents/회사/분기실적발표.pptx','/tmp/cv/home/Documents/회사/예산표.xlsx'],dest:'/tmp/cv/out2'})})"""); time.sleep(4.5)
    def box(name, sel):
        bb = pg.locator(sel).first.bounding_box()
        boxes[name] = bb
    for n, s in [('scope', '#scopeBtn'), ('q', '.qbox'), ('tog', '#togOcr'), ('chips', '#kindChips'), ('row', '#resList .row >> nth=1'),
                 ('pv', '.pv-acts'), ('sel', '#selbar'), ('jobs', '#jobs'), ('nav', '#nav'), ('roots', '#roots'), ('ver', '#verBadge'), ('theme', '#themeToggle'), ('save', '#saveSearch')]:
        try: box(n, s)
        except Exception as e: print('nobox', n, e)
    pg.screenshot(path='/tmp/guide/main.png')
    boxes['navconv'] = pg.locator('#nav [data-view=convert]').bounding_box()
    json.dump(boxes, open('/tmp/guide/boxes.json', 'w'))
    # PDF로 바꾸기
    H = '/tmp/cv/home/Documents'
    pg.evaluate(f"""convertDialog(['{H}/회사/보도자료_누리호.hwp', '{H}/개인/리포트 양식.hwp', '{H}/회사/분기실적발표.pptx', '{H}/회사/예산표.xlsx', '/tmp/cv/home/Desktop/영수증사진.jpg'])"""); time.sleep(1.5)
    pg.click('#cvOpts .switch-row'); time.sleep(0.6)
    pg.locator('#modalBox').screenshot(path='/tmp/guide/convert.png')
    pg.keyboard.press('Escape'); time.sleep(0.4)
    # 옮기기 미리 보기
    pg.click('#selbar [data-act=move]'); time.sleep(0.6); pg.click('.chip[data-p] >> nth=0'); time.sleep(1)
    pg.locator('#modalBox').screenshot(path='/tmp/guide/move.png')
    pg.keyboard.press('Escape'); time.sleep(0.3); pg.keyboard.press('Escape')
    # 분류하기
    pg.click('#nav [data-view=organize]'); time.sleep(1)
    pg.screenshot(path='/tmp/guide/organize.png', clip={'x': 238, 'y': 62, 'width': 1082, 'height': 620})
    # 정리하기 (버전)
    pg.click('#nav [data-view=cleanup]'); time.sleep(0.4); pg.click('.tool[data-k=versions]'); pg.click('#clGo'); time.sleep(2.5)
    pg.screenshot(path='/tmp/guide/cleanup.png', clip={'x': 238, 'y': 62, 'width': 1082, 'height': 620})
    print(json.dumps(boxes)[:400])
    b.close()
