import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

export default function VirtualKeysRedirectPage() {
	const navigate = useNavigate();
	useEffect(() => {
		navigate({ to: "/workspace/config/security/virtual-keys", replace: true });
	}, [navigate]);
	return null;
}