export enum RbacResource {
	VirtualKeys = "VirtualKeys",
	ModelProvider = "ModelProvider",
	Settings = "Settings",
	Logs = "Logs",
	Observability = "Observability",
	MCPGateway = "MCPGateway",
	MCPToolGroups = "MCPToolGroups",
	MCPLogs = "MCPLogs",
	Plugins = "Plugins",
	Dashboard = "Dashboard",
	Governance = "Governance",
	Users = "Users",
	UserProvisioning = "UserProvisioning",
	AuditLogs = "AuditLogs",
	Customers = "Customers",
	Teams = "Teams",
	RBAC = "RBAC",
	RoutingRules = "RoutingRules",
	GuardrailsProviders = "GuardrailsProviders",
	GuardrailsConfig = "GuardrailsConfig",
	CircuitBreaker = "CircuitBreaker",
	Cluster = "Cluster",
	AdaptiveRouter = "AdaptiveRouter",
	FeatureFlags = "FeatureFlags",
	APIKeys = "APIKeys",
	PromptRepository = "PromptRepository",
	SkillsRepository = "SkillsRepository",
	Devices = "Devices",
	Inventory = "Inventory",
	EdgeConfig = "EdgeConfig",
	AccessProfiles = "AccessProfiles",
}

export enum RbacOperation {
	Read = "Read",
	View = "View",
	Create = "Create",
	Update = "Update",
	Delete = "Delete",
	Reveal = "Reveal",
	Download = "Download",
}

export function useRbac(_resource?: RbacResource, _operation?: RbacOperation): boolean {
	return true;
}

// OSS no-op provider: RBAC is always permitted in community builds.
import { createContext, createElement, useContext } from "react";

interface RbacContextValue {
	isLoading: boolean;
}

const RbacContext = createContext<RbacContextValue>({ isLoading: false });

export function RbacProvider({ children }: { children: React.ReactNode }) {
	return createElement(RbacContext.Provider, { value: { isLoading: false } }, children);
}

export function useRbacContext(): RbacContextValue {
	return useContext(RbacContext);
}