package login

func OAuthLink(props any) Node {
	return <a class="button" href={props.href} data-gosx-link>{props.label}</a>
}

func Page() Node {
	return <main id="main-content" class="auth-shell">
		<section class="auth-card">
			<p class="eyebrow">Sign in</p>
			<h1>Rostrum</h1>
			<If cond={data.workspace.readOnlyPreview}>
				<p>
					{data.workspace.previewMessage}
					Sign-in and identity setup are disabled.
				</p>
				<a class="button button-primary" href="/organizer" data-gosx-link>Browse the workspace</a>
			</If>
			<If cond={!data.workspace.readOnlyPreview}>
				<p>
					Sign in with a magic link, a passkey, or a connected account.
				</p>
				<If cond={data.notice.show}>
					<p
						class={"form-status" + (data.notice.kind == "error" ? " form-error" : "")}
						role="status"
						aria-live="polite"
					>{data.notice.message}</p>
				</If>
				<Form class="auth-form" method="post" action="/auth/magic-link">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<input type="hidden" name="next" value={data.next}></input>
					<label>
						<span>Email</span>
						<input
							id="auth-email"
							type="email"
							name="email"
							placeholder="you@example.com"
							autocomplete="email"
							required
						></input>
					</label>
					<p class="form-status" role="status" aria-live="polite"></p>
					<button class="button button-primary" type="submit">Email me a sign-in link</button>
				</Form>
				<div class="auth-divider">
					<span>or</span>
				</div>
				<button
					class="button"
					type="button"
					data-gosx-webauthn-action="authenticate"
					data-gosx-webauthn-options="/auth/webauthn/login-options"
					data-gosx-webauthn-finish="/auth/webauthn/login"
					data-gosx-webauthn-email="#auth-email"
					data-gosx-webauthn-payload={data.webauthnPayload}
					data-gosx-webauthn-status="#auth-passkey-status"
					data-gosx-webauthn-success="/organizer"
				>Sign in with a passkey</button>
				<p id="auth-passkey-status" class="form-status" role="status" aria-live="polite"></p>
				<If cond={data.hasProviders}>
					<div class="auth-divider">
						<span>or continue with</span>
					</div>
					<div class="auth-oauth-list">
						<Each of={data.providers} as="provider">
							<OAuthLink href={provider.href} label={provider.label}></OAuthLink>
						</Each>
					</div>
				</If>
			</If>
		</section>
	</main>
}
