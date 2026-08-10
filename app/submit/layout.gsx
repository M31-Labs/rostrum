package submit

func Layout() Node {
	return <div class="public-flow-shell">
		<header class="public-flow-header">
			<a class="wordmark" href="/" data-gosx-link>
				<span class="wordmark-mark" aria-hidden="true">R</span>
				<span>Rostrum</span>
			</a>
			<span>Speaker application</span>
		</header>
		<Slot />
		<footer class="public-flow-footer">
			<span>Built with GoSX</span>
			<a href="/" data-gosx-link>Privacy & support</a>
		</footer>
	</div>
}
