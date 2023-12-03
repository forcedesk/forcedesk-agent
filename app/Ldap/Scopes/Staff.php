<?php

namespace App\Ldap\Scopes;

use LdapRecord\Models\Model;
use LdapRecord\Models\Scope;
use LdapRecord\Query\Model\Builder;

class Staff implements Scope
{
    /**
     * Apply the scope to the query.
     */
    public function apply(Builder $query, Model $model)
    {
        $group = Group::findOrFail('CN=8707-gs-All Staff,OU=School Groups,OU=Central,DC=services,DC=education,DC=vic,DC=gov,DC=au');

        $query->whereMemberOf($group);
    }
}
