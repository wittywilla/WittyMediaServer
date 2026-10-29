# Performance and security

## Staying light

WittyPDF is used on everything from desktops to TV boxes and old phones, so it's careful with
memory and processing:

- **pdf.js loads only when a PDF is opened.** Apps load WittyPDF lazily, so people who never
  open a PDF never download it.
- **Only nearby pages are drawn:** the pages from about a screen and a half above the window to
  two and a half below it. Pages further away are empty placeholders that keep their size, so the
  scroll bar stays honest.
- **Page images are capped and released.** Each page's canvas is drawn at the screen's pixel
  density (at most double) and limited to 8 million pixels, so zooming in on a large page can't
  exhaust memory. When a page scrolls far away, its canvas is emptied and its text layer removed.
- **Thumbnails** are small (140 pixels wide) and drawn only when they scroll into the list.
- **Large files stream.** Given an address, pdf.js asks for parts of the file (256 KB at a time)
  as pages need them, so a 500-page textbook opens at once when the server supports range
  requests (WittyMediaServer does).
- **Drawing with a pen** updates a single shape as you write rather than redrawing the page, and
  uses the pen's full-resolution input.

## Staying safe

PDFs can contain scripts, forms and fonts that have been used to attack readers. WittyPDF treats
every PDF as untrusted:

- **No PDF scripts:** pdf.js's scripting support is not used, so nothing in a PDF can run code.
- **No XFA forms:** that complex form format is turned off.
- **No `eval`:** fonts are drawn without compiling code (`isEvalSupported: false`).
- **Text is plain text:** search, the text layer and definitions only ever read the page's text;
  nothing from the PDF is inserted into the app as HTML.
- **Built for older engines too:** WittyPDF uses pdf.js's build for older browsers, which also
  lets it run in TV web views and on older devices.
