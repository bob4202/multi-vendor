package httpapi

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"ecommerce/backend/internal/payment"
)

// The fake provider's hosted payment page, for local development. Only
// routed when PAYMENT_PROVIDERS includes "fake". Pay and Decline send a
// signed webhook through the real webhook path, then send the customer
// back to the storefront like Stripe's page would.

var fakePayPage = template.Must(template.New("fake-pay").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Fake payment</title>
<style>
  body { font-family: system-ui, sans-serif; background: #f4f4f5; margin: 0; padding: 16px; color: #18181b; }
  main { max-width: 380px; margin: 48px auto; background: #fff; border-radius: 12px; padding: 24px; box-shadow: 0 1px 3px rgb(0 0 0 / .1); }
  .tag { display: inline-block; font-size: 12px; background: #fef3c7; color: #92400e; padding: 2px 8px; border-radius: 99px; }
  h1 { font-size: 20px; margin: 12px 0 4px; }
  .amount { font-size: 32px; font-weight: 600; margin: 16px 0; }
  p { color: #52525b; margin: 0; }
  button { width: 100%; padding: 12px; border-radius: 8px; border: 1px solid #d4d4d8; background: #fff; font-size: 15px; margin-top: 8px; cursor: pointer; }
  button.pay { background: #18181b; color: #fff; border-color: #18181b; margin-top: 24px; }
</style>
</head>
<body>
<main>
  <span class="tag">Test mode: fake provider</span>
  <h1>Order {{.Reference}}</h1>
  {{if eq .Status "created"}}
    <div class="amount">{{.Amount}}</div>
    <p>No card needed. Pick what the card does.</p>
    <form method="post">
      <input type="hidden" name="success" value="{{.Success}}">
      <input type="hidden" name="cancel" value="{{.Cancel}}">
      <button class="pay" name="action" value="pay">Pay {{.Amount}}</button>
      <button name="action" value="decline">Decline the card</button>
      <button name="action" value="back">Back to the shop</button>
    </form>
  {{else}}
    <p>This payment is {{.Status}}.</p>
    {{if .Cancel}}<form method="post"><input type="hidden" name="cancel" value="{{.Cancel}}"><button name="action" value="back">Back to the shop</button></form>{{end}}
  {{end}}
</main>
</body>
</html>`))

// GET /fake-pay/{ref}?success=...&cancel=...
func (s *Server) handleFakePayPage(w http.ResponseWriter, r *http.Request) {
	info, err := s.payments.FakeCheckoutInfo(r.Context(), chiURLParam(r, "ref"))
	if errors.Is(err, payment.ErrFakePaymentNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	q := r.URL.Query()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = fakePayPage.Execute(w, map[string]string{
		"Reference": info.Reference,
		"Amount":    fmt.Sprintf("£%d.%02d", info.Amount/100, info.Amount%100),
		"Status":    info.Status,
		"Success":   q.Get("success"),
		"Cancel":    q.Get("cancel"),
	})
}

// POST /fake-pay/{ref}  action=pay|decline|back
func (s *Server) handleFakePaySubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ref := chiURLParam(r, "ref")
	back := r.PostForm.Get("cancel")

	var kind payment.EventKind
	switch r.PostForm.Get("action") {
	case "pay":
		kind, back = payment.EventAuthorised, r.PostForm.Get("success")
	case "decline":
		kind = payment.EventFailed
	}
	if kind != "" {
		err := s.payments.SimulateFakeEvent(r.Context(), ref, kind)
		if errors.Is(err, payment.ErrFakePaymentNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
	}

	if u, err := url.Parse(back); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	// No way back (e.g. opened by hand): show the page's new state.
	http.Redirect(w, r, "/fake-pay/"+url.PathEscape(ref), http.StatusSeeOther)
}
