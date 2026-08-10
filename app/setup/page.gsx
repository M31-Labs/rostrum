package setup

func Page() Node {
	return <main id="main-content" class="auth-shell">
		<section class="auth-card">
			<p class="eyebrow">First-time setup</p>
			<h1>Create the first organizer</h1>
			<p>
				This link works once. After you finish, add teammates through ORGANIZER_EMAILS or a future invite — this page never arms again.
			</p>
			<ActionForm class="auth-form" actionName="completeSetup">
				<input type="hidden" name="csrf_token" value={csrf.token}></input>
				<input type="hidden" name="token" value={data.token}></input>
				<p class="form-status" role="alert" aria-live="assertive">{actions.completeSetup.message}</p>
				<label>
					<span>Your email</span>
					<input type="email" name="email" placeholder="you@example.com" required></input>
				</label>
				<label>
					<span>Your name</span>
					<input type="text" name="name" placeholder="Ada Romero"></input>
				</label>
				<button class="button button-primary" type="submit">Finish setup</button>
			</ActionForm>
		</section>
	</main>
}
