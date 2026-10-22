import json, pathlib, re, unittest
ROOT=pathlib.Path(__file__).resolve().parents[1]
class ContentTest(unittest.TestCase):
 def test_catalog(self):
  self.assertTrue((ROOT/'content/book1.json').exists(), '课程目录尚未生成')
  lessons=json.loads((ROOT/'content/book1.json').read_text())['lessons']
  self.assertEqual([x['id'] for x in lessons],list(range(1,145)))
  self.assertEqual(lessons[1]['title'], 'Is this your ...?')
  self.assertEqual(lessons[5]['title'], 'What make is it?')
  for x in lessons:
   self.assertTrue((ROOT/x['audioFile']).is_file(),x['id'])
   self.assertTrue((ROOT/'web'/x['pageImage'].lstrip('/')).is_file(),x['id'])
   self.assertGreater(len(x['lines']),2,x['id'])
   ts=[l['start'] for l in x['lines']]
   self.assertEqual(ts,sorted(set(ts)),x['id'])
   self.assertFalse(any('Listen to the tape' in l['text'] for l in x['lines']),x['id'])
   self.assertEqual(x['audioLesson'],x['id'] if x['id']%2 else x['id']-1)
   self.assertGreater(x['pdfPage'],0)
  self.assertEqual(len(set(x['audioFile'] for x in lessons)),72)
if __name__=='__main__': unittest.main()
