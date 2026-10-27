"use client";

import { useEffect, useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { GoogleSignIn } from "@/components/auth/GoogleSignIn";
import { api } from "@/lib/api";
import { getMe, User } from "@/lib/auth";
import { Challenge, Difficulty, CATEGORY_LABELS, DIFFICULTY_LABELS } from "@/types";
import {
  Play,
  Terminal,
  CheckCircle,
  Clock,
  Box as Docker,
  Boxes as Kubernetes,
  Search,
  Filter,
} from "lucide-react";

export default function Home() {
  const [challenges, setChallenges] = useState<Challenge[]>([]);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState<"all" | "docker" | "kubernetes">("all");
  const [difficulty, setDifficulty] = useState<"all" | Difficulty>("all");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [chals, me] = await Promise.all([
        api.get<{ challenges: Challenge[] }>("/api/challenges"),
        getMe(),
      ]);
      setChallenges(chals.challenges || []);
      setUser(me);
    } catch {
      setChallenges([]);
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const filtered = challenges.filter((c) => {
    const matchesSearch =
      c.title.toLowerCase().includes(search.toLowerCase()) ||
      c.description.toLowerCase().includes(search.toLowerCase());
    const matchesCat = category === "all" || c.category === category;
    const matchesDiff = difficulty === "all" || c.difficulty === difficulty;
    return matchesSearch && matchesCat && matchesDiff;
  });

  const recent = [...challenges].sort((a, b) => {
    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
  }).slice(0, 4);

  const completedCount = user?.credits !== undefined ? 10 - user.credits : 0;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      {/* Header */}
      <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-6 mb-10">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-brand-600 flex items-center justify-center text-white font-bold text-xl">
            K
          </div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight">KubeFix Labs</h1>
            <p className="text-xs text-gray-400">Docker & Kubernetes Troubleshooting</p>
          </div>
        </div>

        <div className="flex items-center gap-3 ml-auto lg:ml-0">
          {user ? (
            <div className="flex items-center gap-3 px-3 py-2 rounded-lg bg-gray-800 border border-gray-700 text-sm">
              <div className="flex flex-col">
                <span className="font-medium">{user.name}</span>
                <span className="text-xs text-gray-400">{user.credits} attempts left</span>
              </div>
              <Button variant="outline" size="sm" onClick={() => {}}>
                Dashboard
              </Button>
            </div>
          ) : (
            <GoogleSignIn
              clientId={process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID || "your-google-client-id"}
              redirectUri={
                process.env.NEXT_PUBLIC_GOOGLE_REDIRECT_URI ||
                "http://localhost:3000/auth/callback"
              }
            />
          )}
          <Button
            variant="brand"
            size="lg"
            disabled={!user}
            onClick={() => (user ? null : window.location.assign("/login"))}
          >
            {!user ? "Sign in to start" : "Start Troubleshooting"}
          </Button>
        </div>
      </div>

      {/* How it works */}
      <div className="mb-10">
        <h2 className="text-lg font-semibold mb-3">How it works</h2>
        <p className="text-gray-400 max-w-2xl">
          KubeFix Labs gives you a real, isolated terminal connected to a broken Docker or
          Kubernetes environment. Diagnose the issue, apply your fix, and submit it for
          automated verification. Each free-tier Google account starts with
          <span className="font-mono text-brand-400">10 challenge-start credits</span>;
          one credit is consumed when a new environment is allocated. Reconnecting to an
          existing attempt does not consume another credit, and incorrect submissions never
          consume start credits.
        </p>
      </div>

      {/* Progress summary */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-10">
        <ProgressCard
          label="Remaining attempts"
          value={user ? Math.max(0, user.credits) : 0}
          icon={Clock}
          subtitle={user ? "Free tier: 10 per account" : "Sign in to begin"}
        />
        <ProgressCard
          label="Completed"
          value={completedCount}
          icon={CheckCircle}
          subtitle="Challenges solved"
        />
        <ProgressCard
          label="Catalog"
          value={challenges.length}
          icon={Docker}
          subtitle="Docker & Kubernetes challenges"
        />
        <ProgressCard
          label="Score"
          value={user ? user.credits * 10 : 0}
          icon={Terminal}
          subtitle="Points earned"
        />
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-4 mb-6">
        <div className="relative w-full md:w-96">
          <Search className="absolute left-3 top-3 h-4 w-4 text-gray-500" />
          <Input
            placeholder="Search challenges…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-10"
          />
        </div>
        <Button variant="outline" size="sm" onClick={() => setCategory("all")}>
          <Filter className="mr-2 h-4 w-4" /> All
        </Button>
        <Button
          variant={category === "docker" ? "brand" : "outline"}
          size="sm"
          onClick={() => setCategory("docker")}
        >
          <Docker className="mr-2 h-4 w-4" /> Docker
        </Button>
        <Button
          variant={category === "kubernetes" ? "brand" : "outline"}
          size="sm"
          onClick={() => setCategory("kubernetes")}
        >
          <Kubernetes className="mr-2 h-4 w-4" /> Kubernetes
        </Button>
        {(["easy", "medium", "hard"] as Difficulty[]).map((d) => (
          <Button
            key={d}
            variant={difficulty === d ? "brand" : "outline"}
            size="sm"
            onClick={() => setDifficulty(difficulty === d ? "all" : d)}
          >
            {DIFFICULTY_LABELS[d]}
          </Button>
        ))}
      </div>

      {filtered.length === 0 && (
        <div className="text-center py-20 text-gray-500">
          <p>No challenges match your filters.</p>
        </div>
      )}

      {/* Recently added */}
      {recent.length > 0 && (
        <section className="mb-10">
          <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Clock className="h-4 w-4" /> Recently added
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {recent.map((c) => (
              <ChallengeCard key={c.id} challenge={c} />
            ))}
          </div>
        </section>
      )}

      {/* All matching challenges */}
      <section>
        <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
          <Terminal className="h-4 w-4" /> Troubleshooting challenges
        </h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {filtered.map((c) => (
            <ChallengeCard key={c.id} challenge={c} />
          ))}
        </div>
      </section>
      {loading && (
        <p role="status" className="mt-6 text-center text-sm text-gray-500">
          Loading challenges…
        </p>
      )}
    </div>
  );
}

function ProgressCard({
  label,
  value,
  icon: Icon,
  subtitle,
}: {
  label: string;
  value: number | string;
  icon: React.ElementType;
  subtitle: string;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-normal text-gray-400">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-end gap-2">
          <span className="text-2xl font-bold text-white">{value}</span>
          <span className="text-xs text-gray-500 pb-1">{subtitle}</span>
        </div>
        <Icon className="w-6 h-6 text-gray-600 ml-2" />
      </CardContent>
    </Card>
  );
}

function ChallengeCard({ challenge }: { challenge: Challenge }) {
  return (
    <Card className="overflow-hidden">
      <CardHeader className="p-4 pb-3 flex justify-between items-start">
        <div>
          <CardTitle className="text-sm">{challenge.title}</CardTitle>
          <Badge
            variant={
              challenge.category === "docker" ? "brand" : challenge.category === "kubernetes"
                ? "default"
                : "default"
            }
            className="mt-2"
          >
            {CATEGORY_LABELS[challenge.category]}
          </Badge>
        </div>
        <Badge variant={challenge.difficulty === "hard" ? "destructive" : "default"}>
          {DIFFICULTY_LABELS[challenge.difficulty]}
        </Badge>
      </CardHeader>
      <CardContent className="p-4 pt-0">
        <p className="text-xs text-gray-400 mb-3 line-clamp-2">
          {challenge.description}
        </p>
        <div className="flex items-center justify-between pt-3 border-t border-gray-800/50">
          <span className="text-xs font-mono text-gray-500">{challenge.points} pts</span>
          <Button variant="outline" size="sm" className="w-9 h-9 p-0">
            <Play className="h-4 w-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
