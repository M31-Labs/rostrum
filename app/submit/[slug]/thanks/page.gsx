package submit

// Page renders the FB-5 success page submitProposal redirects to after a
// proposal is accepted. The heading and body come from the form's
// customizable success copy (domain.SubmissionForm.SuccessPageHeading and
// SuccessPageBody); the page.server.go module also attaches a
// <meta http-equiv="refresh"> head tag that carries the visitor on to their
// keyed speaker portal after about ten seconds, so this page needs no
// script of its own to complete that trip.
func Page() Node {
	return <main id="main-content" class="submission-form-panel">
		<header>
			<p class="panel-kicker">Proposal received</p>
			<h1>{data.form.heading}</h1>
		</header>
		<div class="portal-confirmation">
			<span aria-hidden="true">✓</span>
			<div>
				<If cond={data.submission.title != ""}>
					<strong>{data.submission.title}</strong>
				</If>
				<p>{data.form.body}</p>
			</div>
		</div>
		<If cond={data.hasPortal}>
			<p>
				We will take you to your portal in about 10 seconds.
			</p>
			<a class="button button-primary" href={data.portalURL} data-gosx-link>Go to your portal now</a>
		</If>
		<If cond={!data.hasPortal}>
			<p>
				Check your email for your portal link, or return to the
				<a href={"/submit/" + data.formSlug} data-gosx-link>call for speakers</a>
				.
			</p>
		</If>
	</main>
}
