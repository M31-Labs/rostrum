package app

// Page renders the branded 404: the router's global not-found page
// (M1). It reuses the landing page's shell and the workspace pages' double
// rule (.workspace-header) so a mistyped or stale link lands on a page that
// still reads as Rostrum, not a bare framework error.
func Page() Node {
	return <main id="main-content" class="landing-shell not-found-page">
		<header class="workspace-header">
			<div>
				<p class="eyebrow">404</p>
				<h1>This page has left the programme.</h1>
				<p class="lede">
					The link is out of date, or the page moved. Check the address, or head back to the programme.
				</p>
			</div>
		</header>
		<p class="not-found-return">
			<a class="text-link" href="/" data-gosx-link>
				Page not found — return to the programme
				<span aria-hidden="true">→</span>
			</a>
		</p>
	</main>
}
