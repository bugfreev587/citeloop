import { Workspace } from "./workspace";

export default function ProjectPage({ params }: { params: { id: string } }) {
  return <Workspace projectId={params.id} />;
}
