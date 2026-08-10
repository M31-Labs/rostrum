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
				<a href="/submit/systems-forum-cfp" data-gosx-link>Submit</a>
				<a href="/public/m31-systems-forum-2026/agenda" data-gosx-link>Public agenda</a>
				<a class="button button-small" href="/organizer" data-gosx-link>Open workspace</a>
			</nav>
		</header>
		<Slot />
	</div>
}
