import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

export default function GovernancePage() {
	const navigate = useNavigate();
	useEffect(() => {
		navigate({ to: "/workspace/config/api-keys", replace: true });
	}, [navigate]);
	return null;
}