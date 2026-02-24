Dim doc
Set doc = CreateObject("bpac.Document")

Dim bRet
bRet = doc.Open(WScript.Arguments(0))

If bRet <> False Then
    doc.GetObject("QR").Text = WScript.Arguments(1)
    doc.GetObject("NAME").Text = WScript.Arguments(2)
    doc.GetObject("BARCODE").Text = WScript.Arguments(3)
    doc.GetObject("DATE").Text = WScript.Arguments(4)

    doc.StartPrint "LabelPrinter", 0
    doc.PrintOut 1, 0
    doc.EndPrint
    doc.Close
End If

Set doc = Nothing
