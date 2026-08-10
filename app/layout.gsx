package app

func Layout() Node {
	return <div class="site-shell">
		<a class="skip-link" href="#main-content">Skip to content</a>
		<header class="global-header">
			<a class="wordmark" href="/" data-gosx-link aria-label="Rostrum home">
				<span class="wordmark-mark" aria-hidden="true">R</span>
				<span>Rostrum</span>
			</a>
			<nav class="global-nav" aria-label="Primary">
				<a href={data.workspace.cfpHref} data-gosx-link hidden={!data.workspace.hasCFP}>Submit</a>
				<a href={data.workspace.publicAgendaHref} data-gosx-link>Public agenda</a>
				<a class="button button-small" href="/organizer" data-gosx-link>Open workspace</a>
			</nav>
		</header>
		<Slot />
	</div>
}
