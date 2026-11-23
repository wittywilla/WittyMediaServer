// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyconnect

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

// wantsPage says a browser asked for a page, not a program for data.
func wantsPage(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html")
}

// writePage answers with a page of its own, drawn as the server's standalone pages are (docs/DESIGN.md,
// "Standalone pages"): one card, centred both ways, the app's logo and name at its top, one heading and a sentence
// or two, and nothing that scrolls. Everything is in the page, as the connector serves nothing else: the logo is
// drawn inline and the styles are inline, under a policy that allows nothing more.
func writePage(w http.ResponseWriter, status int, title string, paras ...string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	var b strings.Builder
	for _, p := range paras {
		b.WriteString("<p>" + html.EscapeString(p) + "</p>")
	}
	fmt.Fprintf(w, pageHTML, html.EscapeString(title), html.EscapeString(title), b.String())
}

// The logo is web/public/icon.svg.
const pageHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title>` +
	`<style>:root{color-scheme:dark light;--bg:#121418;--card:#1a1d23;--line:#2b2f38;--fg:#eceef2;--fg2:#b4b9c4}` +
	`@media (prefers-color-scheme:light){:root{--bg:#f4f5f8;--card:#fff;--line:#dfe2e8;--fg:#15171c;--fg2:#4a505c}}` +
	`*{box-sizing:border-box}html,body{margin:0;height:100%%}` +
	`body{display:grid;place-items:center;min-height:100vh;min-height:100dvh;padding:16px;font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:var(--fg);` +
	`background:radial-gradient(70%% 60%% at 50%% 0%%,rgb(99 52 255/.14),transparent 70%%),var(--bg)}` +
	`main{width:min(100%%,30rem);padding:clamp(20px,4vw,36px);border:1px solid var(--line);border-radius:20px;background:var(--card);box-shadow:0 12px 40px rgb(0 0 0/.25);text-align:center}` +
	`.brand{display:flex;align-items:center;justify-content:center;gap:8px;margin-bottom:16px;font-weight:700}.brand svg{width:28px;height:28px}.brand b{color:#9f82ff}` +
	`h1{margin:0 0 8px;font-size:1.5rem;line-height:1.2}p{margin:8px 0 0;color:var(--fg2)}</style></head>` +
	`<body><main role="main"><div class="brand"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024"><defs><linearGradient id="g0" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="1024"><stop offset="0" stop-color="#20242d"/><stop offset="1" stop-color="#13151a"/></linearGradient><radialGradient id="g1" gradientUnits="userSpaceOnUse" cx="512" cy="460.8" r="614.4"><stop offset="0" stop-color="#6334ff" stop-opacity="0.22"/><stop offset="1" stop-color="#6334ff" stop-opacity="0"/></radialGradient><linearGradient id="g2" gradientUnits="userSpaceOnUse" x1="45.52" y1="45.52" x2="978.48" y2="978.48"><stop offset="0" stop-color="#9f82ff"/><stop offset="1" stop-color="#6334ff"/></linearGradient></defs><path fill="url(#g0)" d="M319.08 30.72L704.92 30.72C912.18 30.72 993.28 111.82 993.28 319.08L993.28 704.92C993.28 912.18 912.18 993.28 704.92 993.28L319.08 993.28C111.82 993.28 30.72 912.18 30.72 704.92L30.72 319.08C30.72 111.82 111.82 30.72 319.08 30.72Z"/><path fill="url(#g1)" d="M319.08 30.72L704.92 30.72C912.18 30.72 993.28 111.82 993.28 319.08L993.28 704.92C993.28 912.18 912.18 993.28 704.92 993.28L319.08 993.28C111.82 993.28 30.72 912.18 30.72 704.92L30.72 319.08C30.72 111.82 111.82 30.72 319.08 30.72Z"/><path fill="url(#g2)" d="M906.91 361.81C978.48 548.87 909.52 760.5 741.48 869.49C573.45 978.48 352.09 955.15 210.47 813.53C68.85 671.91 45.52 450.55 154.51 282.52C263.5 114.48 475.13 45.52 662.19 117.09C676.22 122.46 686.33 134.9 688.69 149.74C691.06 164.58 685.33 179.55 673.67 189.02C662 198.49 646.17 201.02 632.14 195.65C482.15 138.27 312.46 193.56 225.08 328.29C137.69 463.02 156.4 640.5 269.95 754.05C383.5 867.6 560.98 886.31 695.71 798.92C830.44 711.54 885.73 541.85 828.35 391.86C820.05 370.17 830.91 345.86 852.61 337.56C874.3 329.26 898.61 340.12 906.91 361.81ZM264.84 417.65C260.77 406.47 264.33 398.85 275.51 394.78L337.79 372.14C348.97 368.07 356.59 371.63 360.65 382.81L412.7 525.93C416.76 537.11 421.73 537.53 427.61 527.18L457.42 474.72C463.29 464.38 472.18 459.21 484.07 459.21L537.03 459.21C548.93 459.21 557.81 464.38 563.69 474.72L593.5 527.18C599.37 537.53 604.34 537.11 608.41 525.93L660.45 382.81C664.52 371.63 672.14 368.07 683.32 372.14L745.6 394.78C756.77 398.85 760.33 406.47 756.27 417.65L666.51 664.48C662.44 675.66 654.49 681.75 642.63 682.74L585.96 687.51C574.11 688.51 565.24 683.84 559.37 673.5L519.37 603.09C513.49 592.75 507.61 592.75 501.74 603.09L461.74 673.5C455.86 683.84 447 688.51 435.14 687.51L378.47 682.74C366.62 681.75 358.66 675.66 354.6 664.48L264.84 417.65Z"/><path fill="#e0a030" d="M853.23 243.11C853.23 283.06 820.84 315.44 780.89 315.44C740.94 315.44 708.56 283.06 708.56 243.11C708.56 203.16 740.94 170.77 780.89 170.77C820.84 170.77 853.23 203.16 853.23 243.11Z"/></svg><span>Witty<b>Media</b>Server</span></div><h1>%s</h1>%s</main></body></html>`
