package portal

func Layout() Node {
	return <div class="portal-shell">
		<header class="portal-header">
			<a class="wordmark" href="/" data-gosx-link>
				<span class="wordmark-mark" aria-hidden="true">P/</span>
				<span>Programma</span>
			</a>
			<nav>
				<a href="#tasks">Tasks</a>
				<a href="#schedule">Schedule</a>
				<a href="#resources">Resources</a>
			</nav>
		</header>
		<Slot />
	</div>
}
