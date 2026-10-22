import Foundation
import PDFKit
import Vision
import AppKit
let args = CommandLine.arguments
let doc = PDFDocument(url: URL(fileURLWithPath: args[1]))!
let out = URL(fileURLWithPath: args[2], isDirectory:true)
try FileManager.default.createDirectory(at:out,withIntermediateDirectories:true)
var rows:[[String:Any]]=[]
for i in 0..<doc.pageCount {
 autoreleasepool {
  let page=doc.page(at:i)!
  let thumb=page.thumbnail(of:NSSize(width:1000,height:1415),for:.mediaBox)
  var rect=NSRect(origin:.zero,size:thumb.size)
  guard let cg=thumb.cgImage(forProposedRect:&rect,context:nil,hints:nil) else {return}
  let req=VNRecognizeTextRequest()
  req.recognitionLevel = .accurate
  req.recognitionLanguages=["en-US"]
  req.usesLanguageCorrection=false
  req.regionOfInterest=CGRect(x:0,y:0.86,width:1,height:0.14)
  do {try VNImageRequestHandler(cgImage:cg,options:[:]).perform([req])} catch { print("OCR ERROR \(i+1): \(error)") }
  let lines=(req.results ?? []).compactMap{$0.topCandidates(1).first?.string}
  let header=lines.joined(separator:" ")
  rows.append(["page":i+1,"header":header])
  let bitmap=NSBitmapImageRep(cgImage:cg)
  if let data=bitmap.representation(using:.jpeg,properties:[.compressionFactor:0.82]) { try? data.write(to:out.appendingPathComponent(String(format:"page-%03d.jpg",i+1))) }
  if i % 20 == 0 {print("Scanned \(i+1)/\(doc.pageCount): \(header)")}
 }
}
let data=try JSONSerialization.data(withJSONObject:rows,options:[.prettyPrinted,.sortedKeys])
try data.write(to:out.appendingPathComponent("headers.json"))
