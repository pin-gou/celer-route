import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LanguageSwitcher } from "@/components/LanguageSwitcher/LanguageSwitcher";
import { ThemeToggle } from "@/components/themeToggle";
import { useBranding } from "@/lib/hooks/useBranding";
import { getErrorMessage, useMemberLoginMutation } from "@/lib/store";
import { GithubLogoIcon } from "@phosphor-icons/react";
import { useNavigate } from "@tanstack/react-router";
import { Eye, EyeOff, Globe } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

const MEMBER_PORTAL_PATH = "/workspace/member";

// Member login view — the /member/login route handler renders this.
// Mirrors the admin loginView pattern (logo + email + password +
// footer) so members see a familiar, branded entry point. Differences:
//
//   - email-not-username input (the user table is keyed on email)
//   - "go to admin sign-in" link in the footer for users who landed
//     here by mistake
//   - admin session is preserved: the server sets a different cookie
//     name (`bf_member_session` vs `token`), so an admin who is also
//     a member can hold both sessions simultaneously.
export default function MemberLoginView() {
	const { t } = useTranslation("governance-ui");
	const { resolvedTheme } = useTheme();
	const [mounted, setMounted] = useState(false);
	const [email, setEmail] = useState("");
	const [password, setPassword] = useState("");
	const [showPassword, setShowPassword] = useState(false);
	const [errorMessage, setErrorMessage] = useState("");
	const [isSubmitting, setIsSubmitting] = useState(false);
	const navigate = useNavigate();
	const [memberLogin] = useMemberLoginMutation();

	useEffect(() => {
		setMounted(true);
	}, []);

	const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		setIsSubmitting(true);
		setErrorMessage("");
		try {
			await memberLogin({ email, password }).unwrap();
			navigate({ to: MEMBER_PORTAL_PATH });
		} catch (error) {
			setErrorMessage(getErrorMessage(error));
		} finally {
			setIsSubmitting(false);
		}
	};

	const { logoSrc, logoAlt } = useBranding(mounted && resolvedTheme === "dark");

	return (
		<div className="flex min-h-screen items-center justify-center p-4">
			<div className="w-full max-w-md">
				<div className="border-border bg-card w-full space-y-6 rounded-sm border p-8">
					{/* Logo */}
					<div className="flex items-center justify-center">
						<img src={logoSrc} alt={logoAlt} width={160} height={26} className="max-h-[40px] w-auto max-w-[220px] object-contain" />
					</div>

					<div className="space-y-2 text-center">
						<h1 className="text-foreground text-lg font-semibold">{t("users.memberLogin.title")}</h1>
						<p className="text-muted-foreground text-sm">{t("users.memberLogin.subtitle")}</p>
					</div>

					<form onSubmit={handleSubmit} className="space-y-5" data-testid="member-login-form">
						{errorMessage && (
							<div className="bg-destructive/10 text-destructive rounded-sm p-3 text-sm" data-testid="member-login-error">
								{errorMessage}
							</div>
						)}

						<div className="space-y-2">
							<Label htmlFor="member-email" className="text-sm font-medium">
								{t("users.memberLogin.email")}
							</Label>
							<Input
								id="member-email"
								type="email"
								placeholder={t("users.memberLogin.emailPlaceholder")}
								value={email}
								onChange={(e) => setEmail(e.target.value)}
								required
								className="text-sm"
								autoComplete="username"
								data-testid="member-login-email"
							/>
						</div>

						<div className="space-y-2">
							<Label htmlFor="member-password" className="text-sm font-medium">
								{t("users.memberLogin.password")}
							</Label>
							<div className="relative">
								<Input
									id="member-password"
									type={showPassword ? "text" : "password"}
									placeholder={t("users.memberLogin.passwordPlaceholder")}
									value={password}
									onChange={(e) => setPassword(e.target.value)}
									required
									className="pr-10 text-sm"
									autoComplete="current-password"
									data-testid="member-login-password"
								/>
								<button
									type="button"
									onClick={() => setShowPassword(!showPassword)}
									className="text-muted-foreground hover:text-foreground absolute top-1/2 right-3 -translate-y-1/2 transition-colors"
									aria-label={showPassword ? t("users.memberLogin.hidePassword") : t("users.memberLogin.showPassword")}
								>
									{showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
								</button>
							</div>
						</div>

						<Button
							type="submit"
							className="h-9 w-full text-sm"
							isLoading={isSubmitting}
							disabled={isSubmitting}
							data-testid="member-login-submit"
						>
							{isSubmitting ? t("users.memberLogin.signingIn") : t("users.memberLogin.signIn")}
						</Button>
					</form>

					{/* Cross-link to admin sign-in — handles the case where a member
					    accidentally lands here, or where an admin wants to switch
					    contexts. Both sessions are independent cookies. */}
					<div className="text-center">
						<a href="/login" className="text-muted-foreground hover:text-primary text-xs underline" data-testid="member-login-admin-link">
							{t("users.memberLogin.adminSignIn")}
						</a>
					</div>

					{/* Footer icons — mirrors the admin login footer. */}
					<div className="flex items-center justify-center gap-4 pt-4">
						<a
							href="https://pin-gou.github.io/celer-route/"
							target="_blank"
							rel="noopener noreferrer"
							className="text-muted-foreground hover:text-primary transition-colors"
							title="Documentation"
						>
							<Globe className="h-5 w-5" />
						</a>
						<a
							href="https://github.com/pin-gou/celer-route"
							target="_blank"
							rel="noopener noreferrer"
							className="text-muted-foreground hover:text-primary transition-colors"
							title={t("users.memberLogin.githubRepository")}
						>
							<GithubLogoIcon className="h-5 w-5" size={22} weight="regular" />
						</a>
						<ThemeToggle />
						<LanguageSwitcher />
					</div>
				</div>
			</div>
		</div>
	);
}