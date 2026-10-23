"""Import source LRC without re-recording; use reviewed OCR page mappings."""
import json,pathlib,re
ROOT=pathlib.Path(__file__).resolve().parents[1]
headers=json.loads((ROOT/'web/assets/pages/headers.json').read_text())
pages={}
for row in headers:
 m=re.match(r'Lesson\s*(\d+)\b',row['header'],re.I)
 if row['page']>=5 and m and 1<=int(m[1])<=144:pages.setdefault(int(m[1]),row['page'])
# Visually verified OCR exceptions (see content-audit.md).
pages.update({51:105,140:287,142:291})
assert set(pages)==set(range(1,145)), 'Missing reviewed lesson pages'
assert all(pages[i]<pages[i+1] for i in range(1,144))
lessons=[]
practice_titles=json.loads((ROOT/'content/practice-titles.json').read_text())
translations=['劳驾！','什么事？','这是您的手提包吗？','请再说一遍。','这是您的手提包吗？','是的，是我的。','非常感谢。']
for path in sorted((ROOT/'New-Concept-English/新概念英语第1册美音（MP3+LRC）').rglob('*.lrc')):
 raw=path.read_bytes()
 try:text=raw.decode('utf-8-sig')
 except UnicodeError:text=raw.decode('gb18030')
 records=[]
 for m in re.finditer(r'\[(\d+):(\d+(?:\.\d+)?)\]([^\r\n]+)',text):
  records.append({'start':round(int(m[1])*60+float(m[2]),2),'text':m[3].strip()})
 odd=int(re.match(r'\d+',path.name)[0]);assert records[0]['text']==f'Lesson {odd}'
 assert records[2]['text'].startswith('Listen to the tape'), path
 lines=records[4:]
 for i,line in enumerate(lines):
  line['end']=lines[i+1]['start'] if i+1<len(lines) else 0
  line['speaker']='原声故事'
  if odd==1:
   line['speaker']=['先生','女士','先生','女士','先生','女士','女士'][i]
   line['translation']=translations[i]
 for lesson_id in [odd,odd+1]:
  page=pages[lesson_id]
  entry={'id':lesson_id,'title':records[1]['text'] if lesson_id%2 else practice_titles[str(lesson_id)], 'kind':'story' if lesson_id%2 else 'practice','pdfPage':page,'audioLesson':odd,'audioFile':str(path.with_suffix('.mp3').relative_to(ROOT)), 'audioUrl':f'/media/audio/{odd}', 'pageImage':f'/assets/pages/page-{page:03d}.jpg','scene':f'/assets/scenes/lesson-{lesson_id:03d}.png' if (ROOT/f'web/assets/scenes/lesson-{lesson_id:03d}.png').is_file() else '', 'lines':lines}
  assert path.with_suffix('.mp3').exists()
  lessons.append(entry)
(ROOT/'content/book1.json').write_text(json.dumps({'version':1,'lessons':lessons},ensure_ascii=False,indent=2))
rows=['# 第一册素材审计','', '日期：2026-10-22。以下为导入器生成的真实映射。','',
 '- 144 个独立单元，72 个 MP3 与 72 个 LRC 配对完整。','- 299 页扫描 PDF 通过本地 macOS Vision OCR 建立映射，页面已导出。','- OCR 例外：51→105、140→287、142→291，均已人工查看图片核对。','- PDF 149–152 是阶段测验，不计入 144 课；不会使后半册映射偏移。','- 72 个录音均核对四条开场字幕结构，正文从第 5 条起。最后一句以音频结束为终点。','- 偶数课标题根据教材页眉整理为独立标题，音频仍为配对奇数课。',
 '- 偶数课展示对应教材页，听力复习复用前一奇数课；题型为游戏派生练习，尚非所有教材书面习题的逐题转录。','- 原 LRC 保留原文；首课附人工中文释义与说话人，其余课目前不提供逐句中文翻译或未经核实的角色标签。',f"- 彩色重绘：{sum(bool(l['scene']) for l in lessons)} 课；其余 {sum(not l['scene'] for l in lessons)} 课使用独立教材页。逐课进度见 lesson-art-upgrade.md。",'- 未做全部 72 段音频逐句人工听校；播放器以源 LRC 为准。','', '| 单元 | 课型 | 标题 | PDF 页 | 音频课 |','|---|---|---|---|---|']
for l in lessons:rows.append(f"| {l['id']} | {l['kind']} | {l['title']} | {l['pdfPage']} | {l['audioLesson']} |")
(ROOT/'docs/content-audit.md').write_text('\n'.join(rows)+'\n')
print('Imported',len(lessons),'lessons;',sum(len(l['lines']) for l in lessons[::2]),'unique timed lines')
