-- Hard-deleting a project must remove every project-scoped record that was
-- created by the early schema before later tables standardized on cascades.

alter table public.product_profiles
  drop constraint if exists product_profiles_project_id_fkey,
  add constraint product_profiles_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;

alter table public.content_inventory
  drop constraint if exists content_inventory_project_id_fkey,
  add constraint content_inventory_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;

alter table public.topics
  drop constraint if exists topics_project_id_fkey,
  add constraint topics_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;

alter table public.articles
  drop constraint if exists articles_project_id_fkey,
  add constraint articles_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;

alter table public.generation_runs
  drop constraint if exists generation_runs_project_id_fkey,
  add constraint generation_runs_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;

alter table public.notification_channels
  drop constraint if exists notification_channels_project_id_fkey,
  add constraint notification_channels_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;

alter table public.notification_subscriptions
  drop constraint if exists notification_subscriptions_project_id_fkey,
  add constraint notification_subscriptions_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;

alter table public.notification_deliveries
  drop constraint if exists notification_deliveries_project_id_fkey,
  add constraint notification_deliveries_project_id_fkey
    foreign key (project_id) references public.projects(id) on delete cascade;
