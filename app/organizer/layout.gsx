package organizer

import "m31labs.dev/gosx/browser"

//gosx:island
func WorkspaceChrome() Node {
	collapsed := signal.NewShared("$rostrumWorkspaceRail", false)
	query := signal.New("")
	needle := signal.Derive(func() string { return query.Get().trim().toLower() })
	routeChord := signal.New(false)
	routeChordAt := signal.New(0)
	routeReady := signal.New(false)
	status := signal.New("")
	toggleRail := func() {
		status.Set(collapsed.Get() ? "Workspace navigation expanded." : "Workspace navigation collapsed.")
		collapsed.Set(!collapsed.Get())
	}
	openCommand := func() {
		query.Set("")
		browser.Open("#workspace-command")
		browser.Focus("#workspace-command-search")
	}
	closeCommand := func() { browser.Close("#workspace-command") }
	closeBackdrop := func() { browser.Close(targetID == currentTargetID, "#workspace-command") }
	filterCommand := func() { query.Set(value) }
	commandKeys := func() {
		browser.PreventDefault(key == "Escape" || key == "ArrowDown" || key == "ArrowUp" || key == "Enter")
		browser.Close(key == "Escape", "#workspace-command")
		browser.FocusMove(key == "ArrowDown", "[data-command-entry]", 1)
		browser.FocusMove(key == "ArrowUp", "[data-command-entry]", -1)
		browser.Activate(key == "Enter", "[data-command-entry]")
	}
	listKeys := func() {
		browser.PreventDefault(key == "ArrowDown" || key == "ArrowUp")
		browser.FocusMove(key == "ArrowDown", "[data-command-entry]", 1)
		browser.FocusMove(key == "ArrowUp", "[data-command-entry]", -1)
	}
	globalKeys := func() {
		routeReady.Set(!editable && !metaKey && !ctrlKey && !altKey && !shiftKey && routeChord.Get() && timeStamp - routeChordAt.Get() <= 1400)
		browser.PreventDefault((!editable && (metaKey || ctrlKey) && key.toLower() == "k") || (!editable && !metaKey && !ctrlKey && !altKey && !shiftKey && (key == "[" || key.toLower() == "g" || routeReady.Get())))
		query.Set(!editable && (metaKey || ctrlKey) && key.toLower() == "k" ? "" : query.Get())
		browser.Open(!editable && (metaKey || ctrlKey) && key.toLower() == "k", "#workspace-command")
		browser.Focus(!editable && (metaKey || ctrlKey) && key.toLower() == "k", "#workspace-command-search")
		browser.Navigate(routeReady.Get() && key.toLower() == "o", "/organizer")
		browser.Navigate(routeReady.Get() && key.toLower() == "f", "/organizer/forms")
		browser.Navigate(routeReady.Get() && key.toLower() == "s", "/organizer/submissions")
		browser.Navigate(routeReady.Get() && key.toLower() == "r", "/organizer/review")
		browser.Navigate(routeReady.Get() && key.toLower() == "p", "/organizer/speakers")
		browser.Navigate(routeReady.Get() && key.toLower() == "a", "/organizer/agenda")
		browser.Navigate(routeReady.Get() && key.toLower() == "c", "/organizer/communications")
		browser.Navigate(routeReady.Get() && key.toLower() == "t", "/organizer/portal")
		browser.Navigate(routeReady.Get() && key.toLower() == "e", "/organizer/embeds")
		browser.Navigate(routeReady.Get() && key.toLower() == "i", "/organizer/integrations")
		browser.Navigate(routeReady.Get() && key.toLower() == "x", "/organizer/settings")
		status.Set(!editable && !metaKey && !ctrlKey && !altKey && !shiftKey && key == "[" ? (collapsed.Get() ? "Workspace navigation expanded." : "Workspace navigation collapsed.") : !editable && !metaKey && !ctrlKey && !altKey && !shiftKey && key.toLower() == "g" ? "Go to… press a workspace route key." : status.Get())
		collapsed.Set(!editable && !metaKey && !ctrlKey && !altKey && !shiftKey && key == "[" ? !collapsed.Get() : collapsed.Get())
		routeChordAt.Set(!editable && !metaKey && !ctrlKey && !altKey && !shiftKey && key.toLower() == "g" ? timeStamp : routeChordAt.Get())
		routeChord.Set(!editable && !metaKey && !ctrlKey && !altKey && !shiftKey && key.toLower() == "g")
	}
	return <div class="workspace-chrome" onDocumentKeyDown={globalKeys}>
		<button
			class="sidebar-collapse"
			type="button"
			aria-controls="workspace-sidebar"
			aria-pressed={collapsed.Get()}
			aria-label={collapsed.Get() ? "Expand navigation" : "Collapse navigation"}
			onClick={toggleRail}
		>
			<span aria-hidden="true">{collapsed.Get() ? "›" : "‹"}</span>
		</button>
		<button
			class="workspace-command-trigger"
			type="button"
			aria-haspopup="dialog"
			aria-controls="workspace-command"
			onClick={openCommand}
		>
			<span class="command-trigger-glyph" aria-hidden="true">⌕</span>
			<span class="command-trigger-label">Quick switcher</span>
			<kbd>Mod K</kbd>
		</button>
		<dialog
			class="command-palette"
			id="workspace-command"
			aria-labelledby="workspace-command-title"
			onClick={closeBackdrop}
		>
			<div class="command-palette-surface">
				<header>
					<div>
						<p class="panel-kicker">Workspace navigation</p>
						<h2 id="workspace-command-title">Go anywhere</h2>
					</div>
					<button class="command-close" type="button" onClick={closeCommand} aria-label="Close quick switcher">×</button>
				</header>
				<label class="command-search">
					<span class="sr-only">Search workspace destinations</span>
					<span aria-hidden="true">⌕</span>
					<input
						id="workspace-command-search"
						type="search"
						placeholder="Search pages, people, or workflows"
						autocomplete="off"
						spellcheck="false"
						value={query.Get()}
						onInput={filterCommand}
						onKeyDown={commandKeys}
					></input>
					<kbd>Esc</kbd>
				</label>
				<nav class="command-list" aria-label="Workspace destinations" onKeyDown={listKeys}>
					<a
						href="/organizer"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"overview home today command center health".contains(needle.Get())}
					>
						<span>Overview</span>
						<kbd>G O</kbd>
					</a>
					<a
						href="/organizer/forms"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"forms routing cfp intake conditional rules".contains(needle.Get())}
					>
						<span>Forms & routing</span>
						<kbd>G F</kbd>
					</a>
					<a
						href="/organizer/submissions"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"submissions proposal inventory applicants status export".contains(needle.Get())}
					>
						<span>Submissions</span>
						<kbd>G S</kbd>
					</a>
					<a
						href="/organizer/review"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"review score rubric evaluation ai reviewer".contains(needle.Get())}
					>
						<span>Review</span>
						<kbd>G R</kbd>
					</a>
					<a
						href="/organizer/speakers"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"speakers people participant profiles readiness".contains(needle.Get())}
					>
						<span>Speakers</span>
						<kbd>G P</kbd>
					</a>
					<a
						href="/organizer/agenda"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"agenda schedule sessions rooms tracks conflicts calendar".contains(needle.Get())}
					>
						<span>Agenda</span>
						<kbd>G A</kbd>
					</a>
					<a
						href="/organizer/communications"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"communications email messages reminder gmail outlook calendar".contains(needle.Get())}
					>
						<span>Communications</span>
						<kbd>G C</kbd>
					</a>
					<a
						href="/organizer/portal"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"portal tasks approvals onboarding wiki resources live".contains(needle.Get())}
					>
						<span>Portal & tasks</span>
						<kbd>G T</kbd>
					</a>
					<a
						href="/organizer/embeds"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"embeds public iframe schedule gallery code".contains(needle.Get())}
					>
						<span>Embeds</span>
						<kbd>G E</kbd>
					</a>
					<a
						href="/organizer/integrations"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"integrations accelevents sync dry run external api".contains(needle.Get())}
					>
						<span>Integrations</span>
						<kbd>G I</kbd>
					</a>
					<a
						href="/organizer/settings"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"settings event configuration timezone venue production".contains(needle.Get())}
					>
						<span>Event settings</span>
						<kbd>G X</kbd>
					</a>
					<a
						href="/submit/systems-forum-cfp"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"new submission submit proposal public cfp".contains(needle.Get())}
					>
						<span>New submission</span>
						<kbd>↵</kbd>
					</a>
					<a
						href="/public/m31-systems-forum-2026/agenda"
						data-gosx-link
						data-command-entry
						hidden={needle.Get() != "" && !"preview public event attendee itinerary agenda".contains(needle.Get())}
					>
						<span>Preview public event</span>
						<kbd>↵</kbd>
					</a>
				</nav>
				<p
					class="command-empty"
					hidden={needle.Get() == "" || "overview home today command center health forms routing cfp intake conditional rules submissions proposal inventory applicants status export review score rubric evaluation ai reviewer speakers people participant profiles readiness agenda schedule sessions rooms tracks conflicts calendar communications email messages reminder gmail outlook portal tasks approvals onboarding wiki resources live embeds public iframe gallery code integrations accelevents sync dry run external api settings event configuration timezone venue production new submit attendee itinerary".contains(needle.Get())}
				>No destination matches that search.</p>
				<footer>
					<span>
						<kbd>↑</kbd>
						<kbd>↓</kbd>
						move
					</span>
					<span>
						<kbd>↵</kbd>
						open
					</span>
					<span>
						<kbd>[</kbd>
						collapse rail
					</span>
				</footer>
			</div>
		</dialog>
		<p class="sr-only" role="status" aria-live="polite">{status.Get()}</p>
	</div>
}

func Layout() Node {
	return <div class="workspace-shell">
		<aside class="workspace-sidebar" id="workspace-sidebar">
			<div class="workspace-event">
				<span class="event-monogram" aria-hidden="true">M31</span>
				<div>
					<strong>M31 Systems Forum</strong>
					<small>Program workspace</small>
				</div>
			</div>
			<nav class="workspace-nav" aria-label="Workspace">
				<a href="/organizer" data-gosx-link aria-current={data.section == "overview" ? "page" : ""}>
					<span aria-hidden="true">⌂</span>
					Overview
				</a>
				<a href="/organizer/forms" data-gosx-link aria-current={data.section == "forms" ? "page" : ""}>
					<span aria-hidden="true">◇</span>
					Forms & routing
				</a>
				<a
					href="/organizer/submissions"
					data-gosx-link
					aria-current={data.section == "submissions" ? "page" : ""}
				>
					<span aria-hidden="true">≡</span>
					Submissions
				</a>
				<a href="/organizer/review" data-gosx-link aria-current={data.section == "review" ? "page" : ""}>
					<span aria-hidden="true">✓</span>
					Review
				</a>
				<a
					href="/organizer/speakers"
					data-gosx-link
					aria-current={data.section == "speakers" ? "page" : ""}
				>
					<span aria-hidden="true">○</span>
					Speakers
				</a>
				<a href="/organizer/agenda" data-gosx-link aria-current={data.section == "agenda" ? "page" : ""}>
					<span aria-hidden="true">▦</span>
					Agenda
				</a>
				<a
					href="/organizer/communications"
					data-gosx-link
					aria-current={data.section == "communications" ? "page" : ""}
				>
					<span aria-hidden="true">↗</span>
					Communications
				</a>
				<a href="/organizer/portal" data-gosx-link aria-current={data.section == "portal" ? "page" : ""}>
					<span aria-hidden="true">□</span>
					Portal & tasks
				</a>
				<a href="/organizer/embeds" data-gosx-link aria-current={data.section == "embeds" ? "page" : ""}>
					<span aria-hidden="true">⌁</span>
					Embeds
				</a>
				<a
					href="/organizer/integrations"
					data-gosx-link
					aria-current={data.section == "integrations" ? "page" : ""}
				>
					<span aria-hidden="true">+</span>
					Integrations
				</a>
			</nav>
			<div class="sidebar-footer">
				<WorkspaceChrome></WorkspaceChrome>
				<a
					href="/organizer/settings"
					data-gosx-link
					aria-current={data.section == "settings" ? "page" : ""}
				>Event settings</a>
				<span>Demo mode · local data</span>
			</div>
		</aside>
		<div class="workspace-main">
			<div class="demo-banner" role="status">
				<span>Interactive demo</span>
				Changes persist locally.
				<Form method="post" action="/demo/reset">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<button type="submit">Reset workspace</button>
				</Form>
			</div>
			<Slot />
		</div>
	</div>
}
