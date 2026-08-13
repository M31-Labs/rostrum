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
		<If cond={data.confirmationStatus == "sent"}>
			<p>
				Your confirmation was accepted by the configured delivery service.
			</p>
		</If>
		<If cond={data.confirmationStatus == "failed"}>
			<p>
				We could not deliver the confirmation just now. Your proposal is safely stored, and the program team can follow up.
			</p>
		</If>
		<If cond={data.confirmationStatus == "enabled"}>
			<p>
				Keep an eye on the email address you submitted for any confirmation and next steps.
			</p>
		</If>
		<If cond={data.hasPortal}>
			<p>
				We will take you to your portal in about 10 seconds.
			</p>
			<a class="button button-primary" href={data.portalURL} data-gosx-link>Go to your portal now</a>
		</If>
		<If cond={!data.hasPortal}>
			<If cond={data.confirmationStatus == "disabled"}>
				<p>Your submission is complete.</p>
			</If>
			<a href={"/submit/" + data.formSlug} data-gosx-link>Return to the call for speakers</a>
		</If>
	</main>
}
