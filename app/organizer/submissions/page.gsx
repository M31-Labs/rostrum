package submissions

func SubmissionStatusForm(props any) Node {
	return <ActionForm class="inline-status-form" actionName="updateStatus">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="submission_id" value={props.id}></input>
		<label>
			<span class="sr-only">
				Status for
				{props.title}
			</span>
			<select name="status" data-gosx-submit-on="change">
				<Each of={data.statusOptions} as="option">
					<option value={option.value} selected={option.value == props.status}>{option.label}</option>
				</Each>
			</select>
			<p class="form-error" data-gosx-field-error="status" aria-live="polite"></p>
		</label>
		<p class="form-status" role="status" aria-live="polite">{action.message}</p>
		<button class="button button-compact" type="submit">Update</button>
	</ActionForm>
}

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Proposal inventory</p>
				<h1>Submissions</h1>
				<p>
					{data.resultCount}
					shown of
					{data.totalCount}
					proposals. Every row retains its routing trace.
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href="/organizer/export/submissions.csv">Export CSV</a>
				<a class="button button-primary" href="/submit/systems-forum-cfp" data-gosx-link>New submission</a>
			</div>
		</header>
		<p class="flash-message">{flash.notice}</p>
		<Form class="filter-bar" method="get" action="/organizer/submissions">
			<label class="search-control">
				<span class="sr-only">Search title or speaker</span>
				<input type="search" name="q" value={data.filters.query} placeholder="Search title or speaker"></input>
			</label>
			<label>
				<span class="sr-only">Filter by status</span>
				<select name="status">
					<Each of={data.filters.statuses} as="status">
						<option value={status.value} selected={status.value == data.filters.status}>{status.label}</option>
					</Each>
				</select>
			</label>
			<label>
				<span class="sr-only">Filter by category</span>
				<select name="category">
					<Each of={data.filters.categories} as="category">
						<option value={category.value} selected={category.value == data.filters.category}>{category.label}</option>
					</Each>
				</select>
			</label>
			<button class="button button-primary" type="submit">Apply filters</button>
			<a class="button button-compact" href="/organizer/submissions" data-gosx-link>Clear</a>
		</Form>
		<section class="panel table-panel">
			<div class="submission-table" role="table" aria-label="Submissions">
				<div class="submission-head" role="row">
					<span>Proposal / speaker</span>
					<span>Program fit</span>
					<span>Routed owner</span>
					<span>Submitted</span>
					<span>Status</span>
				</div>
				<Each of={data.rows} as="row">
					<article class="submission-row" role="row">
						<div class="submission-title" role="cell">
							<span class="avatar avatar-small">{row.initials}</span>
							<div>
								<strong>{row.title}</strong>
								<small>{row.speaker}</small>
							</div>
						</div>
						<div role="cell">
							<span>{row.category}</span>
							<small>
								{row.format}
								·
								{row.level}
							</small>
						</div>
						<div role="cell">
							<strong>{row.owner}</strong>
							<code>{row.queue}</code>
							<details>
								<summary>Why?</summary>
								<p>{row.trace}</p>
							</details>
						</div>
						<span class="mono" role="cell">{row.submitted}</span>
						<div role="cell">
							<span class={"status-pill status-" + row.tone}>{row.status}</span>
							<SubmissionStatusForm id={row.id} title={row.title} status={row.statusValue}></SubmissionStatusForm>
						</div>
					</article>
				</Each>
			</div>
			<If cond={data.resultCount == 0}>
				<div class="empty-state">
					<strong>No proposals match.</strong>
					<p>
						Clear a filter or search for a different title.
					</p>
				</div>
			</If>
		</section>
	</main>
}
