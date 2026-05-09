import React from "react";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { generatePath } from "react-router-dom";
import { CatalogKindDetails } from "./Catalog.KindDetails";
import { CatalogList } from "./Catalog.List";

export const CatalogOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();

  return (
    <Routes>
      <Route index element={<CatalogList />} />
      <Route path="kind/:kindId" element={<CatalogKindDetails />} />
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(
              absoluteRouteMap.children.org.children.catalog.path,
              { orgId },
            )}
          />
        }
      />
    </Routes>
  );
};

export default CatalogOrganization;
